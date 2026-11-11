// cmd/server 进程入口：-config 加载配置 → slog JSON → 雪花单例装配 → App 装配 →
// goroutine 监听 → signal.NotifyContext → 优雅关停（摘流→15s 排空→关连接→退出 0）。
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	_ "time/tzdata" // 内嵌时区库（工控机无 /usr/share/zoneinfo 也可解析，防时间漂移）

	"github.com/cool-wx/sys-iam-go/internal/app"
	"github.com/cool-wx/sys-iam-go/internal/config"
	"github.com/cool-wx/sys-iam-go/pkg/ids"
)

func main() {
	configPath := flag.String("config", "./config.yaml", "配置文件路径（默认工作目录 ./config.yaml）")
	flag.Parse()

	// 判定 -config 是否显式指定（决定文件缺失时是 warn+默认值还是致命退出）
	explicit := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "config" {
			explicit = true
		}
	})

	// slog JSON handler → stdout（journald 接管）
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: parseLevel(config.DefaultLogLevel)}))
	slog.SetDefault(logger)

	cfg, err := config.Load(*configPath, explicit)
	if err != nil {
		slog.Error("load config failed", "error", err.Error())
		os.Exit(1)
	}
	// 重设日志级别（配置加载后）
	logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: parseLevel(cfg.Log.Level)}))
	slog.SetDefault(logger)

	// 雪花节点进程单例装配（A4：单机 workerId 来自配置）
	node, err := ids.NewNode(cfg.Snowflake.Node)
	if err != nil {
		slog.Error("create snowflake node failed", "error", err.Error())
		os.Exit(1)
	}
	ids.SetDefault(node)

	application, err := app.New(cfg, logger)
	if err != nil {
		slog.Error("init app failed", "error", err.Error())
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- application.Run()
	}()

	select {
	case err := <-serveErr:
		// 端口占用等真实启动失败 → 非 0 退出
		if err != nil {
			slog.Error("server exited", "error", err.Error())
			os.Exit(1)
		}
		return
	case <-ctx.Done():
		// SIGINT/SIGTERM → 摘流 → 15s 排空 → 关连接 → 正常退出
		application.Stop()
	}
}

// parseLevel 配置字符串 → slog 级别（未知值按 info）。
func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
