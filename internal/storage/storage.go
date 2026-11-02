// Package storage 提供文件存储抽象：FileStorage 三方法
// 接口 + Local/S3 双实现，storage.driver 配置切换（local|s3），同一套上传/下载/
// 删除业务代码对两种后端无差别。
//
// 设计决策：
//   - 接口面按源 File 域实际用到的最小面收敛（createBucket/upload/download/
//     delete 四能力 → Put/GetStream/Delete 三方法；createBucket 收编进 S3Storage
//     构造内 EnsureBucket——等价源 @PostConstruct createBucket 启动语义）。
//   - local 模式延迟形态：storage.New 的 local 分支只构造 LocalStorage，
//     不触碰 minio-go 任何符号——工控机 local 部署零 S3 连接、minio 依赖不参与
//     构造链；driver=s3 时启动 EnsureBucket fail-fast（桶不可建则拒绝启动）。
//   - key 语义：恒为服务端生成（uuid + "_" + 原文件名），不做客户端输入面；但
//     LocalStorage 仍做路径逃逸防御（防御纵深，local.go resolveKey）。
//
// 双实现的语义对齐点：
//   - Delete 幂等宽容：对象不存在均返回 nil（os.IsNotExist 宽容 / S3 RemoveObject
//     天然幂等）——宽容删除语义的存储侧基座；
//   - GetStream 返回 (reader, size, error)，size 供 HTTP Content-Length（
//     流式下载不整载）；S3 侧先 StatObject 再 GetObject——GetObject 的错误在读流
//     时才浮现（minio 文档明示），Stat 前置让 404 在响应头写出前暴露。
package storage

import (
	"context"
	"fmt"
	"io"

	"github.com/cool-wx/sys-iam-go/internal/config"
)

// FileStorage 文件存储抽象接口：Put/GetStream/Delete 三方法。
//
// Put 的 size 参数是已知真实值（multipart.FileHeader.Size），必须直传存储——
// minio PutObject 传 -1 会按分片上传且每片缓冲于内存（文档明示），工控机内存
// 场景禁用（prohibition）。
type FileStorage interface {
	// Put 写入对象：key 为服务端生成的对象键，r 为内容流，size 为真实字节数，
	// contentType 为 MIME 类型（空串由存储侧自行兜底）。
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error

	// GetStream 读取对象：返回内容流与对象字节数（供 Content-Length 头）。
	// 调用方负责 Close。对象不存在返回错误（下载链前置校验 DB 记录，此处
	// 错误语义为存储侧真实缺失）。
	GetStream(ctx context.Context, key string) (io.ReadCloser, int64, error)

	// Delete 删除对象：幂等宽容——对象不存在返回 nil（宽容语义基座，
	// 删除链存储失败仅告警不阻断 DB 清理，双实现对齐）。
	Delete(ctx context.Context, key string) error
}

// New 按 driver 分派构造 FileStorage：local → LocalStorage（零 minio
// 触碰）；s3 → S3Storage（构造内 EnsureBucket，失败 fail-fast 上抛——照 app.go
// enforcer/Redis Ping 先例，存储基建不可用绝不静默启动）。
func New(ctx context.Context, cfg config.StorageConfig) (FileStorage, error) {
	switch cfg.Driver {
	case config.StorageDriverLocal:
		return NewLocalStorage(cfg.Local.Root), nil
	case config.StorageDriverS3:
		return NewS3Storage(ctx, cfg.S3)
	default:
		// config.Load 已 fail-fast 非法 driver；此分支为防御编程直接调用兜底。
		return nil, fmt.Errorf("storage: unknown driver %q (must be local|s3)", cfg.Driver)
	}
}
