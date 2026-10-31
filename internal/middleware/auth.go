package middleware

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/cool-wx/sys-iam-go/internal/auth"
	"github.com/cool-wx/sys-iam-go/pkg/errcode"
	"github.com/cool-wx/sys-iam-go/pkg/resp"
)

// bearerPrefix Authorization 头的标准前缀（D-04：token 仅经 Bearer 传递，
// X-API-Key 等 header 方式不实现）。
const bearerPrefix = "Bearer "

// BearerToken 从 Authorization 头提取 Bearer token：前缀不符、缺失或 token 空白
// 均返回 ok=false。认证中间件与 logout handler 共用同一解析规则。
func BearerToken(header string) (string, bool) {
	if !strings.HasPrefix(header, bearerPrefix) {
		return "", false
	}
	token := strings.TrimSpace(header[len(bearerPrefix):])
	if token == "" {
		return "", false
	}
	return token, true
}

// Auth 认证中间件：解析 Bearer token → SessionManager.Resolve（命中即续期）→
// Identity 注入请求 ctx（repo 审计钩子经 auth.FromContext 读取）。中间件不判路径
// ——白名单在 router 分组解决（pub 组不挂本中间件）。
//
// 错误三分支（T-2-04 fail-closed）：
//   - 无 token/前缀不符/会话不存在 → 401（ErrUnauthenticated，AUTH-06）
//   - Redis 故障等其他错误 → 500 Abort，绝不静默 c.Next() 放行
func Auth(sm *auth.SessionManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := BearerToken(c.GetHeader("Authorization"))
		if !ok {
			resp.Fail(c, errcode.ErrUnauthenticated)
			c.Abort()
			return
		}
		identity, err := sm.Resolve(c.Request.Context(), token)
		if errors.Is(err, auth.ErrSessionNotFound) {
			resp.Fail(c, errcode.ErrUnauthenticated)
			c.Abort()
			return
		}
		if err != nil {
			// fail-closed：Redis 故障时认证中间件不得放行请求（T-2-04 高危缓解），
			// 500 由全局错误语义收敛为 ErrInternal 固定文案，不回显底层错误。
			resp.Fail(c, errcode.ErrInternal)
			c.Abort()
			return
		}
		c.Request = c.Request.WithContext(auth.WithIdentity(c.Request.Context(), identity))
		c.Next()
	}
}
