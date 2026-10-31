// Package app 聚合应用依赖与生命周期：装配（config→GORM→路由）→ Run 阻塞服务 →
// Stop 摘流+排空+关连接（供 main 信号路径与关停测试复用，P16/P5 范式）。
package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/cool-wx/sys-iam-go/internal/auth"
	"github.com/cool-wx/sys-iam-go/internal/config"
	"github.com/cool-wx/sys-iam-go/internal/handler"
	"github.com/cool-wx/sys-iam-go/internal/health"
	"github.com/cool-wx/sys-iam-go/internal/middleware"
	"github.com/cool-wx/sys-iam-go/internal/repository"
	"github.com/cool-wx/sys-iam-go/internal/service"
)

// 优雅关停排空预算（P16；systemd TimeoutStopSec 须大于此值，Pattern 5）。
const shutdownTimeout = 15 * time.Second

// App 应用聚合：engine、db、rdb、health、config。
type App struct {
	cfg    *config.Config
	logger *slog.Logger
	engine *gin.Engine
	db     *gorm.DB
	rdb    *redis.Client
	health *health.State
	srv    *http.Server
}

// New 装配全部依赖：GORM 连接（绝不 AutoMigrate，schema 沿用源库）、连接池
// 工控机参数、Redis 会话存储（Ping fail-fast）、健康状态、认证路由。成功返回可
// Run 的 App。
func New(cfg *config.Config, logger *slog.Logger) (*App, error) {
	db, err := gorm.Open(mysql.Open(cfg.MySQL.DSN), &gorm.Config{
		Logger:                 gormlogger.Default.LogMode(gormlogger.Warn),
		SkipDefaultTransaction: true, // service 层显式 Transaction，关隐式事务
		TranslateError:         true, // MySQL 1062 → gorm.ErrDuplicatedKey（P4）
		// 不配置 AutoMigrate —— schema 沿用源库（CLAUDE.md 锁定）
	})
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get sql db: %w", err)
	}
	sqlDB.SetMaxOpenConns(cfg.MySQL.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MySQL.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.MySQL.ConnMaxLifetime)

	h := health.New(sqlDB)

	// Redis 会话存储（go-redis v9；启动 Ping fail-fast 对齐 app.New 的 err 返回风格
	// ——会话基建不可用时服务绝不能以"全量 401"状态静默启动）
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	err = rdb.Ping(pingCtx).Err()
	cancel()
	if err != nil {
		return nil, fmt.Errorf("ping redis: %w", err)
	}

	sessionMgr := auth.NewSessionManager(rdb, cfg.Auth.SessionTimeout)

	userRepo := repository.NewUserRepo(db)
	relationRepo := repository.NewRelationRepo(db)
	apiKeyRepo := repository.NewApiKeyRepo(db)
	menuRepo := repository.NewMenuRepo(db)
	deptRepo := repository.NewDeptRepo(db)
	roleRepo := repository.NewRoleRepo(db)
	userSvc := service.NewUserService(userRepo)
	authSvc := service.NewAuthService(userRepo, relationRepo, apiKeyRepo, sessionMgr)
	menuSvc := service.NewMenuService(menuRepo)
	deptSvc := service.NewDeptService(deptRepo)
	roleSvc := service.NewRoleService(db, roleRepo, menuRepo, relationRepo)
	userHandler := handler.NewUserHandler(userSvc)
	authHandler := handler.NewAuthHandler(authSvc)
	menuHandler := handler.NewMenuHandler(menuSvc)
	deptHandler := handler.NewDeptHandler(deptSvc)
	roleHandler := handler.NewRoleHandler(roleSvc)

	engine := newRouter(h, handlers{
		Auth:  authHandler,
		Users: userHandler,
		Menus: menuHandler,
		Depts: deptHandler,
		Roles: roleHandler,
	}, middleware.Auth(sessionMgr), logger)

	return &App{
		cfg:    cfg,
		logger: logger,
		engine: engine,
		db:     db,
		rdb:    rdb,
		health: h,
		srv: &http.Server{
			Addr:    fmt.Sprintf(":%d", cfg.Server.Port),
			Handler: engine,
			// 传输层超时（WR-02/WR-05/T-1-04）：三个超时各封一段——
			//   - ReadHeaderTimeout=10s 首道闸：封 slowloris 慢头（工控机 fd 有限，
			//     头迟迟不完整即断）；
			//   - ReadTimeout=30s 封整个请求（头+体）的读取总时长——体积上限
			//     （BodyLimit 1MB）只封内存不封时间，若无此界，头在 10s 内合法发完的
			//     客户端可按每秒 1 字节滴漏 body，连接与 goroutine 被无限期占用
			//     （slowloris 变体）。30s 对工控机局域网极宽裕：1MB JSON 在百兆内网
			//     <1s 传完，正常请求远触不到上界；ReadTimeout 只约束读取，不影响
			//     响应写出；
			//   - IdleTimeout=120s 回收 keep-alive 空闲连接。
			// 刻意不设 WriteTimeout：响应无大 payload，保留长响应兼容性。
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       30 * time.Second,
			IdleTimeout:       120 * time.Second,
		},
	}, nil
}

// Run 阻塞监听服务；http.ErrServerClosed 视为正常返回 nil。
func (a *App) Run() error {
	a.logger.Info("server listening", "port", a.cfg.Server.Port)
	if err := a.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Stop 优雅关停：先摘流（readyz 立即 503）→ 15s 排空在途请求 → 关 Redis/MySQL 连接。
func (a *App) Stop() {
	a.health.SetNotReady()
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := a.srv.Shutdown(ctx); err != nil {
		a.logger.Error("http shutdown", "error", err.Error())
	}
	if sqlDB, err := a.db.DB(); err == nil {
		_ = sqlDB.Close()
	}
	if err := a.rdb.Close(); err != nil {
		a.logger.Error("redis close", "error", err.Error())
	}
	a.logger.Info("shutdown complete")
}

// Engine 暴露 gin engine（供集成测试 httptest 复用真实装配，不经网络监听）。
func (a *App) Engine() *gin.Engine {
	return a.engine
}
