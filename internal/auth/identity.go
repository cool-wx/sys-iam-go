// Package auth 定义请求身份（identity）在 context 中的传播。
// 等价源 UserContext ThreadLocal → context.Context；私有类型 key 防跨包冲突
// （ARCHITECTURE.md Pattern 2 / Anti-Pattern 4）。
package auth

import "context"

// Identity 请求操作者身份：由认证中间件注入（Phase 1 无中间件注入，D-02
// 白名单直通；本包供测试注入与 Phase 2 复用）。
type Identity struct {
	UserID int64
	Name   string
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
