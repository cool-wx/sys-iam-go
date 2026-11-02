// Package auth 定义请求身份（identity）在 context 中的传播。
// 等价源 UserContext ThreadLocal → context.Context；私有类型 key 防跨包冲突
// （ARCHITECTURE.md Pattern 2 / Anti-Pattern 4）。
package auth

import "context"

// Identity 请求操作者身份：由认证中间件注入（白名单路径直通无注入；
// 本包亦供测试注入）。Roles 为登录时会话内的角色码
// 快照（授权 gate 依据——RequireRoles 按此判定），可能为 nil（无角色用户）。
type Identity struct {
	UserID int64
	Name   string
	Roles  []string
}

// ctxKey 私有类型 key——绝不用 string key。
type ctxKey struct{}

// WithIdentity 将身份注入 context。
func WithIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// FromContext 取出身份；无 identity 时 ok=false（后台任务语义：审计操作人保持 NULL）。
func FromContext(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(ctxKey{}).(Identity)
	return id, ok
}
