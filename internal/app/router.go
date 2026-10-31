package app

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/cool-wx/sys-iam-go/internal/handler"
	"github.com/cool-wx/sys-iam-go/internal/health"
	"github.com/cool-wx/sys-iam-go/internal/middleware"
)

// handlers 路由聚合（Wave 2/3 扩域时按域增字段，newRouter 签名不再反复变化）。
type handlers struct {
	Auth  *handler.AuthHandler
	Users *handler.UserHandler
}

// newRouter 装配路由：四中间件全局链（RequestID→AccessLog→Recovery→BodyLimit）+
// 认证分组（D-15/AUTH-06 白名单收紧）：
//   - pub 组（免 token）：/healthz、/readyz、POST /api/v1/auth/login、
//     POST /api/v1/auth/apikey-login（白名单四路径齐备）；
//   - pri 组（authMw 全量校验）：POST /api/v1/auth/logout、POST /api/v1/auth/kickout
//     + 既有 users 四路由。
func newRouter(h *health.State, hs handlers, authMw gin.HandlerFunc, logger *slog.Logger) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(middleware.RequestID())
	engine.Use(middleware.AccessLog(logger))
	engine.Use(middleware.Recovery(logger))
	engine.Use(middleware.BodyLimit()) // 请求体 1MB 上限（WR-02/T-1-04 DoS 防护）

	pub := engine.Group("/")
	pub.GET("/healthz", h.Liveness)
	pub.GET("/readyz", h.Readiness)
	pub.POST("/api/v1/auth/login", hs.Auth.Login)
	pub.POST("/api/v1/auth/apikey-login", hs.Auth.ApikeyLogin)

	pri := engine.Group("/", authMw)
	pri.POST("/api/v1/auth/logout", hs.Auth.Logout)
	pri.POST("/api/v1/auth/kickout", hs.Auth.Kickout)
	pri.POST("/api/v1/users", hs.Users.Create)
	pri.PUT("/api/v1/users/:id", hs.Users.Update)
	pri.GET("/api/v1/users/:id", hs.Users.Get)
	pri.GET("/api/v1/users", hs.Users.List)

	return engine
}
