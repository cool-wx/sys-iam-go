//go:build integration

package storage_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cool-wx/sys-iam-go/internal/config"
	"github.com/cool-wx/sys-iam-go/internal/storage"
)

// s3TestConfig S3 smoke 测试端点/凭证（env 可覆盖；默认值为本机 rustfs 容器
// 实证值 localhost:9000 admin/admin——不锚定容器名，只探测端口，环境自适应：
// 与镜像探测先例同构；CI/其他机器不可达即 skip）。
func s3TestConfig(t *testing.T) config.S3Config {
	t.Helper()
	return config.S3Config{
		Endpoint:  envOr(t, "IAM_TEST_S3_ENDPOINT", "localhost:9000"),
		AccessKey: envOr(t, "IAM_TEST_S3_ACCESS_KEY", "admin"),
		SecretKey: envOr(t, "IAM_TEST_S3_SECRET_KEY", "admin"),
		Bucket:    "iamtest-" + randomSuffix(),
	}
}

func envOr(t *testing.T, key, def string) string {
	t.Helper()
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func randomSuffix() string {
	buf := make([]byte, 4)
	_, err := rand.Read(buf)
	if err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

// requireS3Reachable 2 秒 TCP 探测：不可达 t.Skip（smoke 不阻塞——容器不可达是
// 环境状态非代码缺陷；FAIL 只能来自可达时的行为断言失败）。探测前剥离 scheme
// （env 覆盖值可带 http:// 前缀，Dial 只收裸 host:port）。
func requireS3Reachable(t *testing.T, endpoint string) {
	t.Helper()
	host := strings.TrimPrefix(strings.TrimPrefix(endpoint, "https://"), "http://")
	conn, err := net.DialTimeout("tcp", host, 2*time.Second)
	if err != nil {
		t.Skipf("s3 endpoint %s unreachable, smoke skipped: %v", endpoint, err)
	}
	_ = conn.Close()
}

func TestS3StorageSmoke(t *testing.T) {
	// S3 真容器 smoke（rustfs Up 时真跑 / 不可达 skip）：EnsureBucket
	// 建桶幂等 → Put 随机 1KB → GetStream 字节一致 → Delete → StatObject 确认
	// 消失。测试桶独立随机前缀（iamtest-{rand}）不污染默认桶，结束后清理。
	cfg := s3TestConfig(t)
	requireS3Reachable(t, cfg.Endpoint)

	ctx := context.Background()
	s, err := storage.NewS3Storage(ctx, cfg)
	require.NoError(t, err, "NewS3Storage 构造含 EnsureBucket 必须成功（桶自动创建）")
	defer func() {
		// 清理测试桶（RemoveBucket 前先删尽对象——smoke 只留 1 对象）。
		_ = s.Delete(ctx, "smoke.bin")
		mc, _ := minio.New(cfg.Endpoint, &minio.Options{
			Creds: credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		})
		if mc != nil {
			_ = mc.RemoveBucket(ctx, cfg.Bucket)
		}
	}()

	// EnsureBucket 幂等（构造已建桶，二次调用成功即幂等证）。
	s2, err := storage.NewS3Storage(ctx, cfg)
	require.NoError(t, err, "EnsureBucket 幂等：桶已存在时二次构造必须成功")
	assert.NotNil(t, s2)

	content := make([]byte, 1024)
	_, err = rand.Read(content)
	require.NoError(t, err)

	require.NoError(t, s.Put(ctx, "smoke.bin", bytes.NewReader(content), int64(len(content)), "application/octet-stream"),
		"Put 真实 size 直传（禁 -1 分片内存缓冲，prohibition）")

	r, size, err := s.GetStream(ctx, "smoke.bin")
	require.NoError(t, err)
	assert.Equal(t, int64(1024), size, "StatObject 前置返回对象字节数")
	got := make([]byte, 1024)
	_, err = io.ReadFull(r, got)
	require.NoError(t, err)
	require.NoError(t, r.Close())
	assert.Equal(t, content, got, "S3 往返内容逐字节一致")

	require.NoError(t, s.Delete(ctx, "smoke.bin"), "删除成功")
	_, _, err = s.GetStream(ctx, "smoke.bin")
	require.Error(t, err, "删除后 GetStream（StatObject 前置）必须报错——404 在读流前暴露（坑 2）")
}
