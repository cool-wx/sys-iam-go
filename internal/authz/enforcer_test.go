// enforcer 纯内存单元测试（model.NewModelFromString + 无 adapter，免容器毫秒级
// ——03-RESEARCH 研究重点 4 测试策略；gorm-adapter 路径由集成测试覆盖）。
// 锁定 matcher 三坑：token 名一致性、super_admin 豁免首位短路（零 p 行直证）、
// 撤销 g 行豁免即时失效（D-07），以及 SyncRolePerms 幂等全量替换语义。
package authz

import (
	"testing"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newMemEnforcer 无 adapter 内存 enforcer（普通 Enforcer 足够——单 goroutine 单测）。
func newMemEnforcer(t *testing.T) *casbin.Enforcer {
	t.Helper()
	m, err := model.NewModelFromString(rbacModel)
	require.NoError(t, err, "内嵌 model 串必须可解析")
	e, err := casbin.NewEnforcer(m)
	require.NoError(t, err)
	return e
}

// newMemSynced 构造被测包装（SyncRolePerms 走真实 SyncedEnforcer 内存实例）。
func newMemSynced(t *testing.T) *Enforcer {
	t.Helper()
	m, err := model.NewModelFromString(rbacModel)
	require.NoError(t, err)
	inner, err := casbin.NewSyncedEnforcer(m)
	require.NoError(t, err)
	return &Enforcer{inner: inner}
}

// TestMatcherTokenNames matcher token 名与 policy_definition 一致（坑 1 回归锁）：
// p = role, perm 定义下常规 g+p 三元组命中——token 名写错时 model 加载或 Enforce
// 直接报错（No parameter 'p_sub' found.），本测试恒锁 p.role/p.perm 拼写。
func TestMatcherTokenNames(t *testing.T) {
	e := newMemEnforcer(t)
	_, err := e.AddGroupingPolicy("1001", "dept_admin")
	require.NoError(t, err)
	_, err = e.AddPolicy("dept_admin", "system:dept:list")
	require.NoError(t, err)

	allowed, err := e.Enforce("1001", "system:dept:list")
	require.NoError(t, err)
	assert.True(t, allowed, "g+p 三元组必须命中（matcher token 名一致性直证）")

	// perm 不匹配 → false（模型仍然健康，非 error 路径）
	allowed, err = e.Enforce("1001", "system:user:list")
	require.NoError(t, err)
	assert.False(t, allowed)
}

// TestSuperAdminExemptWithoutPolicies super_admin 豁免独立于 p 行（坑 2 回归锁，
// T-3-03 critical 缓解直证）：仅 g 行、零 p 行时任意 perm 均 true——豁免必须是
// matcher 首个 || 短路分支，请求不匹配任何 p 行时 matcher 也必须被求值。
func TestSuperAdminExemptWithoutPolicies(t *testing.T) {
	e := newMemEnforcer(t)
	_, err := e.AddGroupingPolicy("9001", "super_admin")
	require.NoError(t, err)

	for _, perm := range []string{"system:user:list", "system:anything:whatever", "unknown:domain:act"} {
		allowed, err := e.Enforce("9001", perm)
		require.NoError(t, err)
		assert.True(t, allowed, "零 p 行 super_admin 必须豁免放行: perm=%s", perm)
	}

	// 非 super_admin 用户零 p 行 → false（豁免不外溢）
	allowed, err := e.Enforce("9002", "system:user:list")
	require.NoError(t, err)
	assert.False(t, allowed, "无 g 行用户零 p 行必须拒绝")
}

// TestRevokeSuperAdminImmediate 撤销即时性（D-07 唯一判据直证）：删除 super_admin
// g 行后同 sub 的 Enforce 下一调用即 false——豁免判据完全走 g 层，非会话快照。
func TestRevokeSuperAdminImmediate(t *testing.T) {
	e := newMemEnforcer(t)
	_, err := e.AddGroupingPolicy("9001", "super_admin")
	require.NoError(t, err)

	allowed, err := e.Enforce("9001", "system:user:list")
	require.NoError(t, err)
	require.True(t, allowed, "撤销前必须豁免放行")

	_, err = e.RemoveFilteredGroupingPolicy(0, "9001")
	require.NoError(t, err)

	allowed, err = e.Enforce("9001", "system:user:list")
	require.NoError(t, err)
	assert.False(t, allowed, "撤销 super_admin g 行后豁免必须下一调用即失效")
}

// TestHasRoleGLayer HasRole g 层读原语直证（CR-04）：g 行存在 → true；删 g 行后
// 下一调用即 false——判据源是 enforcer 实时策略而非任何快照（回收即刻生效，D-07
// 同轴）；未持有角色 / 无 g 行用户 → false 且无 error。
func TestHasRoleGLayer(t *testing.T) {
	e := newMemSynced(t)
	_, err := e.inner.AddGroupingPolicy("9001", "super_admin")
	require.NoError(t, err)
	_, err = e.inner.AddGroupingPolicy("9001", "editor")
	require.NoError(t, err)

	has, err := e.HasRole(9001, "super_admin")
	require.NoError(t, err)
	assert.True(t, has, "g 行存在时 HasRole 必须 true")

	has, err = e.HasRole(9001, "viewer")
	require.NoError(t, err)
	assert.False(t, has, "g 行存在但角色未持有必须 false")

	// 回收（删 g 行）后下一调用即 false——CR-04 判据实时性直证
	_, err = e.inner.RemoveFilteredGroupingPolicy(0, "9001")
	require.NoError(t, err)
	has, err = e.HasRole(9001, "super_admin")
	require.NoError(t, err)
	assert.False(t, has, "回收 g 行后 HasRole 下一调用即 false（判据不得读快照）")

	has, err = e.HasRole(9002, "super_admin")
	require.NoError(t, err)
	assert.False(t, has, "无 g 行用户必须 false 且无 error")
}

// TestSyncRolePermsReplacesAll SyncRolePerms 幂等全量替换直证：先手工两行 p 策略，
// SyncRolePerms 单行替换后该角色 GetPolicy 恰一行（旧两行被清）。
func TestSyncRolePermsReplacesAll(t *testing.T) {
	e := newMemSynced(t)
	_, err := e.inner.AddPolicies([][]string{
		{"ops_role", "system:user:list"},
		{"ops_role", "system:user:add"},
	})
	require.NoError(t, err)

	require.NoError(t, e.SyncRolePerms("ops_role", []string{"system:user:edit"}))

	policies, err := e.inner.GetPolicy()
	require.NoError(t, err)
	require.Len(t, policies, 1, "SyncRolePerms 必须全删旧 p 行再全加")
	assert.Equal(t, []string{"ops_role", "system:user:edit"}, policies[0])

	// 空 perms=只删不加（全回收语义）
	require.NoError(t, e.SyncRolePerms("ops_role", nil))
	policies, err = e.inner.GetPolicy()
	require.NoError(t, err)
	assert.Empty(t, policies, "空 perms 必须清空该角色全部 p 行")
}
