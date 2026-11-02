//go:build integration

// app 优雅关停集成测试（双重证明之一：进程内证明）。
// 两个测试合计覆盖验收断言 (a)-(d)：
//   - TestReadinessFlipBeforeDrain：摘流可观察性——真实 handler + 真实 MySQL ping 上
//     SetNotReady()（App.Stop 摘流步骤）后 readyz 立即 503（确定性单步直证）；
//   - TestGracefulShutdownDrains：App.Stop() 全路径——慢请求完整返回、15s 预算内、
//     排空窗口内探针永不 200（摘流先于排空的强断言）、Stop 后新连接拒绝。
//
// 技术注记：Go http.Server.Shutdown 并发执行时立即关闭 idle 连接并停止 accept，
// "排空窗口内外部观察到 readyz 503"的观察窗口仅为 SetNotReady→closeIdleConns 的
// 微秒级竞态，不可稳定断言；摘流翻面以单步直证（503），排空窗口的摘流效果以
// "StopStart 之后无任何 200"强断言覆盖（503 与连接拒绝均为"不 ready"的合法表现，
// 强断言覆盖其全部表现——严格强于仅断言 503）。
package app_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/cool-wx/sys-iam-go/internal/app"
	"github.com/cool-wx/sys-iam-go/internal/config"
	"github.com/cool-wx/sys-iam-go/internal/health"
	"github.com/cool-wx/sys-iam-go/internal/testutil"
)

// slowSleep 慢路由 handler 睡眠时长：模拟进行中请求（> 摘流观察窗口、< 15s 排空预算）。
const slowSleep = 1200 * time.Millisecond

// inFlightPause 发起慢请求后到 Stop 之前的等待：确保请求已进入 handler sleep（在途）。
const inFlightPause = 300 * time.Millisecond

// newShutdownApp 起独立 MySQL+Redis 双容器（装配链含 Redis Ping fail-fast）
// + 完整真 App 装配 + 真实端口监听（本组测试不经 httptest 驱动 App 生命周期——断言
// 的是 http.Server.Shutdown 真实行为）。注册测试专用慢路由（仅测试进程内注册，
// 生产路由零污染；engine 直挂不经 pri 组，不受 Auth 中间件约束）。
// 返回 baseURL 与 App；cleanup 链：Stop（幂等保险）→ 容器终止。
func newShutdownApp(t *testing.T) (string, *app.App) {
	t.Helper()
	ctx := context.Background()
	dsn, stopMySQL, err := testutil.RunMySQL(ctx)
	require.NoError(t, err, "mysql testcontainer 启动与 DDL 引导失败")
	redisAddr, stopRedis, err := testutil.RunRedis(ctx)
	require.NoError(t, err, "redis testcontainer 启动失败")

	// 取空闲端口再交还（App.Run 真实 ListenAndServe）
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()

	cfg := &config.Config{
		Server:    config.ServerConfig{Port: port},
		MySQL:     config.MySQLConfig{DSN: dsn, MaxOpenConns: 5, MaxIdleConns: 2, ConnMaxLifetime: time.Minute},
		Redis:     config.RedisConfig{Addr: redisAddr},
		Auth:      config.AuthConfig{SessionTimeout: time.Hour},
		Snowflake: config.SnowflakeConfig{Node: 1},
		Log:       config.LogConfig{Level: "error"},
		// File 域：app.New 无条件构造 FileStorage——测试 cfg 需 local driver
		// + 独立临时根目录（与 handler 包 newTestApp 同构最小装配）
		Storage: config.StorageConfig{
			Driver: "local",
			Local:  config.LocalStorageConfig{Root: t.TempDir()},
		},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	application, err := app.New(cfg, logger)
	require.NoError(t, err, "app 装配失败")

	// 测试专用慢路由：进行中请求载体（生产路由零污染）
	application.Engine().GET("/__test__/slow", func(c *gin.Context) {
		time.Sleep(slowSleep)
		c.JSON(http.StatusOK, gin.H{"status": "slow-done"})
	})

	go func() { _ = application.Run() }()
	t.Cleanup(func() {
		application.Stop()
		stopRedis()
		stopMySQL()
	})

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	// 等待真实监听就绪（/healthz 200）
	require.Eventually(t, func() bool {
		resp, err := http.Get(baseURL + "/healthz")
		if err != nil {
			return false
		}
		drainBody(resp)
		return true
	}, 5*time.Second, 20*time.Millisecond, "服务真实监听就绪（/healthz 200）")
	return baseURL, application
}

// newPooledClient 独立连接池 client（不污染全局 DefaultTransport）。
func newPooledClient() *http.Client {
	return &http.Client{Transport: &http.Transport{}, Timeout: 10 * time.Second}
}

// drainBody 读完并关闭响应体（连接可复用的前提）。
func drainBody(resp *http.Response) {
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
}

// TestReadinessFlipBeforeDrain 摘流可观察性直证（探针禁令："收到 SIGTERM 后
// /readyz MUST NOT 继续报告 ready"）：真实 health.Readiness handler + 真实 MySQL ping
// 上，SetNotReady()（App.Stop 摘流步骤）之前 200、之后立即 503——摘流翻面的
// 探针效果 100% 确定性证明（"先摘流再排空"的"摘流"一侧）。
func TestReadinessFlipBeforeDrain(t *testing.T) {
	ctx := context.Background()
	dsn, stopContainer, err := testutil.RunMySQL(ctx)
	require.NoError(t, err, "mysql testcontainer 启动与 DDL 引导失败")
	t.Cleanup(stopContainer)

	checkDB, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	require.NoError(t, err)
	sqlDB, err := checkDB.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	// 真实 handler 挂载（与生产 /readyz 同一处理函数、同一 MySQL ping 路径）
	h := health.New(sqlDB)
	engine := gin.New()
	engine.GET("/readyz", h.Readiness)
	ts := httptest.NewServer(engine)
	t.Cleanup(ts.Close)

	// 基线：摘流前 readyz 200（真实 ping 成功）
	resp, err := http.Get(ts.URL + "/readyz")
	require.NoError(t, err)
	drainBody(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode, "摘流前 readyz 应 200")

	// 摘流单步（App.Stop() 的第一步即 health.SetNotReady）
	h.SetNotReady()

	// 摘流后 readyz 立即 503（MUST NOT 继续报告 ready）
	resp, err = http.Get(ts.URL + "/readyz")
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode, "摘流后 readyz 必须 503")
	assert.JSONEq(t, `{"status":"fail"}`, string(body), "503 body 仅含 status 键")
}

// TestGracefulShutdownDrains App.Stop() 全路径排空证明：慢请求在途时调用 Stop()——
// (a) 慢请求完整收到 200 响应（进行中请求不截断）；(b) Stop 在 15s 排空预算内返回；
// (c) 排空开始后 readyz 永不返回 200（摘流先于排空完成）；(d) Stop 后新连接被拒绝。
func TestGracefulShutdownDrains(t *testing.T) {
	baseURL, application := newShutdownApp(t)

	// 慢请求（进行中请求）
	type slowResult struct {
		status int
		body   string
		err    error
	}
	slowCh := make(chan slowResult, 1)
	go func() {
		client := newPooledClient()
		resp, err := client.Get(baseURL + "/__test__/slow")
		if err != nil {
			slowCh <- slowResult{err: err}
			return
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		slowCh <- slowResult{status: resp.StatusCode, body: string(b)}
	}()
	time.Sleep(inFlightPause) // 确保慢请求在途（已进入 handler sleep）

	// readyz 观察洪流：Stop 前后持续高频观察（覆盖整个摘流+排空窗口）
	type observation struct {
		at   time.Time
		code int // 0 = 连接错误（含 Stop 后拒绝）
	}
	var mu sync.Mutex
	var observations []observation
	floodStop := make(chan struct{})
	go func() {
		client := newPooledClient()
		for {
			select {
			case <-floodStop:
				return
			default:
			}
			code := 0
			r, err := client.Get(baseURL + "/readyz")
			if err == nil {
				drainBody(r)
				code = r.StatusCode
			}
			mu.Lock()
			observations = append(observations, observation{at: time.Now(), code: code})
			mu.Unlock()
			if err != nil {
				time.Sleep(2 * time.Millisecond) // 拒绝后限频空转
			}
		}
	}()

	// SIGTERM 等价全路径：摘流 → 15s 排空 → 关连接
	stopStart := time.Now()
	application.Stop()
	elapsed := time.Since(stopStart)
	close(floodStop)

	// (a) 进行中请求完整收到响应（排空语义核心）
	res := <-slowCh
	require.NoError(t, res.err, "进行中慢请求必须完整收到响应而非连接错误")
	require.Equal(t, http.StatusOK, res.status, "进行中慢请求必须完整 200（不截断）")
	assert.Contains(t, res.body, "slow-done")

	// (b) Stop() 无返回值（实际 API 形态）；排空预算以耗时断言：< 15s
	assert.Less(t, elapsed, 15*time.Second, "Stop 必须在 15s 排空预算内返回，实测 %s", elapsed)

	// (c) 摘流先于排空完成：排空窗口内探针永不 ready（不得出现任何 200；
	// 503 与连接拒绝均为"不 ready"的合法表现）
	mu.Lock()
	require.NotEmpty(t, observations, "readyz 观察洪流应有记录")
	afterStart, postStart200, saw503 := 0, 0, false
	for _, o := range observations {
		if !o.at.Before(stopStart) {
			afterStart++
			switch o.code {
			case http.StatusOK:
				postStart200++
			case http.StatusServiceUnavailable:
				saw503 = true
			}
		}
	}
	mu.Unlock()
	require.Greater(t, afterStart, 0, "观察洪流必须覆盖 Stop 之后的时间窗")
	assert.Zero(t, postStart200, "排空开始后 readyz 不得再返回 200（摘流先于排空）")
	if saw503 {
		t.Log("增强证据：排空窗口内直接观察到 readyz 503")
	}

	// (d) Stop 后新连接被拒绝（listener 已关闭）
	if _, connErr := http.Get(baseURL + "/healthz"); connErr == nil {
		t.Fatal("Stop 后新连接必须被拒绝（listener 应已关闭）")
	}
}
