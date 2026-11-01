package app

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/cool-wx/sys-iam-go/internal/authz"
	"github.com/cool-wx/sys-iam-go/internal/handler"
	"github.com/cool-wx/sys-iam-go/internal/health"
	"github.com/cool-wx/sys-iam-go/internal/middleware"
)

// handlers 路由聚合（Wave 2/3 扩域时按域增字段，newRouter 签名不再反复变化）。
type handlers struct {
	Auth    *handler.AuthHandler
	Users   *handler.UserHandler
	Menus   *handler.MenuHandler
	Depts   *handler.DeptHandler
	Roles   *handler.RoleHandler
	ApiKeys *handler.ApiKeyHandler
}

// newRouter 装配路由：四中间件全局链（RequestID→AccessLog→Recovery→BodyLimit）+
// 认证分组（D-15/AUTH-06 白名单收紧）：
//   - pub 组（免 token）：/healthz、/readyz、POST /api/v1/auth/login、
//     POST /api/v1/auth/apikey-login（白名单四路径齐备）；
//   - pri 组（authMw 全量校验）：POST /api/v1/auth/logout、POST /api/v1/auth/kickout
//     （kickout 额外挂 RequireRoles("super_admin") 授权 gate，WR-05 过渡收口）
//   - 既有 users 四路由 + menus/depts 全套业务路由（02-02 D-11/D-13：复数资源
//     路由 + 树端点独立；gin 静态段 tree 与 :id 参数段共存，静态优先）。
func newRouter(h *health.State, hs handlers, authMw gin.HandlerFunc, az authz.Authorizer, logger *slog.Logger) *gin.Engine {
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
	// kickout 授权收口（WR-05/T-2-07 过渡态）：仅 super_admin 可调（会话 roles
	// 快照判定）——不再"任何有效 token 可踢任意会话（含 admin）"；其余管理端点
	// 的 requirePerm 全量收口在 03-03（D-15：kickout 的 RequireRoles 保留叠加）
	pri.POST("/api/v1/auth/kickout", middleware.RequireRoles("super_admin"), hs.Auth.Kickout)
	pri.POST("/api/v1/users", hs.Users.Create)
	pri.PUT("/api/v1/users/:id", hs.Users.Update)
	pri.GET("/api/v1/users/:id", hs.Users.Get)
	// requirePerm tracer 首路由（D-08 逐路由显式挂载，路由与权限同屏）：casbin
	// RBAC 判定（用户 g 行→角色→p 行 perm 串），权限变更下一请求即时生效（PERM-02）
	pri.GET("/api/v1/users", middleware.RequirePerm(az, authz.PermUserList), hs.Users.List)
	pri.DELETE("/api/v1/users/:id", hs.Users.Delete)
	// 用户装配子资源（USER-04/02/03/07，02-03）：roles 全量替换、status 停用踢会话、
	// reset-password 重置踢会话——gin 静态段（roles/status/reset-password）与 :id
	// 参数段共存，静态优先（与 roles/:id/menus 同构）
	pri.GET("/api/v1/users/:id/roles", hs.Users.GetRoles)
	pri.PUT("/api/v1/users/:id/roles", hs.Users.AssignRoles)
	pri.PUT("/api/v1/users/:id/status", hs.Users.UpdateStatus)
	pri.PUT("/api/v1/users/:id/reset-password", hs.Users.ResetPassword)

	// 菜单域（MENU-01/02）：CRUD + 树端点（静态段 tree 与 :id 参数段共存）
	pri.POST("/api/v1/menus", hs.Menus.Create)
	pri.PUT("/api/v1/menus/:id", hs.Menus.Update)
	pri.GET("/api/v1/menus/tree", hs.Menus.Tree)
	pri.GET("/api/v1/menus/:id", hs.Menus.Get)
	pri.GET("/api/v1/menus", hs.Menus.List)
	pri.DELETE("/api/v1/menus/:id", hs.Menus.Delete)

	// 部门域（DEPT-01/02）：与菜单域同构
	pri.POST("/api/v1/depts", hs.Depts.Create)
	pri.PUT("/api/v1/depts/:id", hs.Depts.Update)
	pri.GET("/api/v1/depts/tree", hs.Depts.Tree)
	pri.GET("/api/v1/depts/:id", hs.Depts.Get)
	pri.GET("/api/v1/depts", hs.Depts.List)
	pri.DELETE("/api/v1/depts/:id", hs.Depts.Delete)

	// 角色域（ROLE-01/02/03）：CRUD + 角色菜单分配子资源（全量替换语义）
	pri.POST("/api/v1/roles", hs.Roles.Create)
	pri.PUT("/api/v1/roles/:id", hs.Roles.Update)
	pri.GET("/api/v1/roles/:id", hs.Roles.Get)
	pri.GET("/api/v1/roles", hs.Roles.List)
	pri.DELETE("/api/v1/roles/:id", hs.Roles.Delete)
	pri.GET("/api/v1/roles/:id/menus", hs.Roles.GetMenus)
	pri.PUT("/api/v1/roles/:id/menus", hs.Roles.AssignMenus)
	// 数据权限范围部门子资源（PERM-03 档位 4 存储面，D-10 sys_role_dept）：独立
	// 子资源端点（与 roles/:id/menus 同形态，管理面先例是全删重建）；RequirePerm
	// 挂载照 03-01 tracer 先例（D-08 逐路由显式，其余路由 03-03 全量收口）
	pri.GET("/api/v1/roles/:id/scopes", middleware.RequirePerm(az, authz.PermRoleQuery), hs.Roles.GetScopes)
	pri.PUT("/api/v1/roles/:id/scopes", middleware.RequirePerm(az, authz.PermRoleEdit), hs.Roles.AssignScopes)

	// API Key 域（AKEY-01/02，02-03）：CRUD 五端点 + status 子资源（源 editApiKey
	// 仅 status 单列语义）。至此 Phase 2 五域路由全量注册（D-11 六组齐备）
	pri.POST("/api/v1/api-keys", hs.ApiKeys.Create)
	pri.GET("/api/v1/api-keys/:id", hs.ApiKeys.Get)
	pri.GET("/api/v1/api-keys", hs.ApiKeys.List)
	pri.PUT("/api/v1/api-keys/:id/status", hs.ApiKeys.UpdateStatus)
	pri.DELETE("/api/v1/api-keys/:id", hs.ApiKeys.Delete)

	return engine
}
