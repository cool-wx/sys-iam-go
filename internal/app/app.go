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
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/cool-wx/sys-iam-go/internal/config"
	"github.com/cool-wx/sys-iam-go/internal/handler"
	"github.com/cool-wx/sys-iam-go/internal/health"
	"github.com/cool-wx/sys-iam-go/internal/repository"
	"github.com/cool-wx/sys-iam-go/internal/service"
)

// 优雅关停排空预算（P16；systemd TimeoutStopSec 须大于此值，Pattern 5）。
const shutdownTimeout = 15 * time.Second

// App 应用聚合：engine、db、health、config。
type App struct {
	cfg    *config.Config
	logger *slog.Logger
	engine *gin.Engine
	db     *gorm.DB
	health *health.State
	srv    *http.Server
}

// New 装配全部依赖：GORM 连接（绝不 AutoMigrate，schema 沿用源库）、连接池
// 工控机参数、健康状态、路由。成功返回可 Run 的 App。
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

	userRepo := repository.NewUserRepo(db)
	userSvc := service.NewUserService(userRepo)
	userHandler := handler.NewUserHandler(userSvc)

	engine := newRouter(h, userHandler, logger)

	return &App{
		cfg:    cfg,
		logger: logger,
		engine: engine,
		db:     db,
		health: h,
		srv: &http.Server{
			Addr:    fmt.Sprintf(":%d", cfg.Server.Port),
			Handler: engine,
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

// Stop 优雅关停：先摘流（readyz 立即 503）→ 15s 排空在途请求 → 关 MySQL 连接。
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
	a.logger.Info("shutdown complete")
}

// Engine 暴露 gin engine（供集成测试 httptest 复用真实装配，不经网络监听）。
func (a *App) Engine() *gin.Engine {
	return a.engine
}
