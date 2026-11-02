// s3.go S3Storage 实现：minio-go v7.4.1（锁定版本，
// 纯 Go 无 CGO），对接任意 S3 兼容端点（工控机现场为 RustFS）。
//
// 两个 minio 集成坑（04-RESEARCH 重点 4 实证，改动前必读）：
//  1. PutObject 的 size 必须传真实值——传 -1 会按 multipart 分片且每片缓冲于
//     内存（文档明示），工控机禁用；调用方（FileService 上传链）持 multipart
//     FileHeader.Size 真实值直传；
//  2. GetObject 的错误在读流时才浮现（含对象不存在的 404）——GetStream 先
//     StatObject 取 size 与存在性，404 在响应头写出前暴露，避免"已写 200 头
//     后流中断"的半截响应。
package storage

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/cool-wx/sys-iam-go/internal/config"
)

// S3Storage S3 兼容对象存储实现（minio-go）。
type S3Storage struct {
	client *minio.Client
	bucket string
}

// NewS3Storage 构造并 EnsureBucket fail-fast：BucketExists 为假则 MakeBucket
// （幂等可重入），失败上抛——等价源 @PostConstruct createBucket 启动语义
// （FileApplicationService.java:45-48），存储基建不可用绝不静默启动。
//
// endpoint 规范化：配置值沿用源默认形态可带 scheme（http://localhost:9000，
// RustfsProperties.java），而 minio.New 要求裸 host:port——此处剥离 scheme 且
// https:// 联动 Secure=true（显式 secure 配置仍优先覆盖 http:// 的 false 语义）。
func NewS3Storage(ctx context.Context, cfg config.S3Config) (*S3Storage, error) {
	endpoint := cfg.Endpoint
	secure := cfg.Secure
	switch {
	case strings.HasPrefix(endpoint, "https://"):
		endpoint = strings.TrimPrefix(endpoint, "https://")
		secure = true
	case strings.HasPrefix(endpoint, "http://"):
		endpoint = strings.TrimPrefix(endpoint, "http://")
	}
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: secure,
	})
	if err != nil {
		return nil, fmt.Errorf("storage: new s3 client: %w", err)
	}
	s := &S3Storage{client: client, bucket: cfg.Bucket}
	if err := s.EnsureBucket(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

// EnsureBucket 启动期桶保障（BucketExists → MakeBucket 幂等）。
func (s *S3Storage) EnsureBucket(ctx context.Context) error {
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("storage: check bucket %q: %w", s.bucket, err)
	}
	if !exists {
		if err := s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{}); err != nil {
			return fmt.Errorf("storage: create bucket %q: %w", s.bucket, err)
		}
	}
	return nil
}

// Put 写入对象：size 直传 PutObject（prohibition：真实值，禁 -1 分片内存缓冲）。
func (s *S3Storage) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	_, err := s.client.PutObject(ctx, s.bucket, key, r, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return fmt.Errorf("storage: put object %q: %w", key, err)
	}
	return nil
}

// GetStream 读取对象：先 StatObject 取 size 与存在性（坑 2：GetObject 错误延迟
// 到读流，Stat 前置让 404 在调用方写响应头之前暴露），再 GetObject 返回
// *minio.Object（实现 io.ReadCloser）。
func (s *S3Storage) GetStream(ctx context.Context, key string) (io.ReadCloser, int64, error) {
	info, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return nil, 0, fmt.Errorf("storage: stat object %q: %w", key, err)
	}
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, 0, fmt.Errorf("storage: open object %q: %w", key, err)
	}
	return obj, info.Size, nil
}

// Delete 删除对象：RemoveObject 天然幂等（对象不存在成功返回），与 LocalStorage
// 的 IsNotExist 宽容语义对齐（双实现基座）。
func (s *S3Storage) Delete(ctx context.Context, key string) error {
	if err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("storage: remove object %q: %w", key, err)
	}
	return nil
}
