package middleware

import (
	"log/slog"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/cool-wx/sys-iam-go/internal/auth"
	"github.com/cool-wx/sys-iam-go/internal/authz"
	"github.com/cool-wx/sys-iam-go/pkg/errcode"
	"github.com/cool-wx/sys-iam-go/pkg/resp"
)

// RequirePerm 功能权限 gate（逐路由显式挂载、路由与权限同屏可审计）。
// 判定走 enforcer（casbin RBAC：g 行 用户→角色 + p 行 角色→perm），
// 权限分配/回收后下一请求立即生效——显式区别于 RequireRoles 的登录
// 快照语义。必须挂在 Auth 之后（依赖 identity 注入）。
//
// 错误三分支（fail-closed，形态照 RequireRoles）：
//   - identity 缺失 → 401（防御性分支，正常装配不可达）；
//   - enforcer 故障 → 500 Abort 绝不放行（fail-closed，认证中间件同款纪律——
//     enforcer 故障不得变成 fail-open）；
//   - 未命中 → 403/10200 ErrForbidden Abort（与 RequireRoles 同码）。
//
// super_admin 豁免不在中间件判断——model matcher 首位短路已覆盖（唯一
// 判据在 enforcer g 层：撤销 super_admin g 行后豁免下一请求立即失效）。
//
// enforcer 故障分支必须 slog.Error 留痕：授权链故障是 IAM 的 P0 事件，
// 表现为全站无差别 500（每条请求都过 RequirePerm），AccessLog 只有 status 行——
// 无根因日志则 casbin/gorm-adapter/DB 故障完全不可诊断。日志键仅 sub（userID 十进
// 制串）/perm（权限串）/error（根因）三键，不含 token/请求体等敏感值。
func RequirePerm(az authz.Authorizer, perm string) gin.HandlerFunc {
	return func(c *gin.Context) {
		identity, ok := auth.FromContext(c.Request.Context())
		if !ok {
			resp.Fail(c, errcode.ErrUnauthenticated)
			c.Abort()
			return
		}
		allowed, err := az.Enforce(strconv.FormatInt(identity.UserID, 10), perm)
		if err != nil {
			slog.Error("enforcer enforce failed",
				"sub", strconv.FormatInt(identity.UserID, 10),
				"perm", perm,
				"error", err)
			resp.Fail(c, errcode.ErrInternal)
			c.Abort()
			return
		}
		if !allowed {
			resp.Fail(c, errcode.ErrForbidden)
			c.Abort()
			return
		}
		c.Next()
	}
}
