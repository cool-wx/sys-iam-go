//go:build integration

// Redis 集成测试容器基建（与 mysql.go 同构：独立容器 + cleanup + 镜像离线自适应）。
// D-10/D-12 铁律：自动化测试一律 testcontainers 独立容器，不锚定本机具名容器。
package testutil

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
)

// defaultRedisImage 标准镜像 tag（有网环境由 testcontainers 拉取）。
const defaultRedisImage = "redis:8"

// RunRedis 启动独立 redis 容器（无密码、无 db 前缀约束），返回 go-redis Options.Addr
// 形态的 "host:port"（模块 ConnectionString 的 redis:// scheme 已剥离）与 cleanup
// （终止容器）。会话管理器/认证中间件集成测试共用。
func RunRedis(ctx context.Context) (string, func(), error) {
	maybeDisableRyuk()
	ctr, err := tcredis.Run(ctx, redisTestImage())
	if err != nil {
		return "", nil, fmt.Errorf("start redis container: %w", err)
	}

	cs, err := ctr.ConnectionString(ctx)
	if err != nil {
		terminateRedis(ctr)
		return "", nil, fmt.Errorf("connection string: %w", err)
	}
	addr := strings.TrimPrefix(cs, "redis://")

	cleanup := func() { terminateRedis(ctr) }
	return addr, cleanup, nil
}

// terminateRedis 终止容器（独立 ctx：测试 ctx 可能已取消/超时）。
func terminateRedis(ctr *tcredis.RedisContainer) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = ctr.Terminate(ctx)
}

// redisTestImage 决定集成测试 Redis 镜像（复刻 mysql testImage 三级探测）：
//  1. IAM_TEST_REDIS_IMAGE 环境变量显式指定；
//  2. 否则探测本地已有的 redis:8.x 镜像（离线环境无 Docker Hub 访问，直接复用本地
//     缓存，避免无效拉取重试拖慢测试）；
//  3. 兜底 redis:8（有网环境由 testcontainers 正常拉取）。
func redisTestImage() string {
	if v := os.Getenv("IAM_TEST_REDIS_IMAGE"); v != "" {
		return v
	}
	if img := localRedisImage(); img != "" {
		return img
	}
	return defaultRedisImage
}

// localRedisImage 返回本地已缓存的最新 redis:8.x 镜像 tag（无 docker CLI 或无匹配则空串）。
func localRedisImage() string {
	dockerBin, err := exec.LookPath("docker")
	if err != nil {
		return ""
	}
	out, err := exec.Command(dockerBin, "images", "--format", "{{.Repository}}:{{.Tag}}").Output()
	if err != nil {
		return ""
	}
	best := ""
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "redis:8") || line == "redis:8<none>" {
			continue
		}
		if line > best {
			best = line
		}
	}
	return best
}
