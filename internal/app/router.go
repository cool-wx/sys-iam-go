package app

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/cool-wx/sys-iam-go/internal/handler"
	"github.com/cool-wx/sys-iam-go/internal/health"
	"github.com/cool-wx/sys-iam-go/internal/middleware"
)

// newRouter 装配路由：三中间件全局链（RequestID→AccessLog→Recovery，顺序固定）+
// public 组（Phase 1 全白名单直通，D-02 临时态；Phase 2 加 Auth 中间件时收紧）。
func newRouter(h *health.State, userHandler *handler.UserHandler, logger *slog.Logger) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(middleware.RequestID())
	engine.Use(middleware.AccessLog(logger))
	engine.Use(middleware.Recovery(logger))

	pub := engine.Group("/")
	pub.GET("/healthz", h.Liveness)
	pub.GET("/readyz", h.Readiness)
	pub.POST("/api/v1/users", userHandler.Create)

	return engine
}
