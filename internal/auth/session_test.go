// SessionManager 单元测试（miniredis 纯 Go 内存 Redis，无 build tag 免 Docker）。
// TTL 不会自动流逝——用 FastForward 显式推进时间，确定性断言续期与过期语义。
package auth

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestManager 起独立 miniredis + 构造 SessionManager（timeout 30min 供常规用例）。
func newTestManager(t *testing.T) (*SessionManager, *miniredis.Miniredis, *redis.Client) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return NewSessionManager(rdb, 30*time.Minute), mr, rdb
}

// TestSessionLoginResolve 登录→校验往返：token 为 64 hex 字符（256-bit 熵）；
// Resolve 返回正确 Identity——含 roles JSON 往返与 realName 非空白优先/空白回退
// userName（审计操作人取值语义）。
func TestSessionLoginResolve(t *testing.T) {
	sm, mr, _ := newTestManager(t)
	ctx := context.Background()

	token, err := sm.Login(ctx, SessionPayload{
		UserID: 42, UserName: "admin", RealName: "管理员", Roles: []string{"super_admin", "ops"},
	})
	require.NoError(t, err)
	require.Len(t, token, 64, "token 必须为 64 hex 字符（crypto/rand 32B）")
	for _, ch := range token {
		assert.True(t, (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f'), "token 含非 hex 字符 %q", ch)
	}

	id, err := sm.Resolve(ctx, token)
	require.NoError(t, err)
	assert.Equal(t, int64(42), id.UserID)
	assert.Equal(t, "管理员", id.Name, "realName 非空白优先")
	assert.Equal(t, []string{"super_admin", "ops"}, id.Roles, "roles 快照必须经 Resolve 往返（RequireRoles 授权判定依据）")

	// 会话 Hash 载荷四键齐备（userId/userName/realName/roles）+ roles JSON 往返
	vals := mr.HGet(keySessionPrefix+token, "roles")
	assert.JSONEq(t, `["super_admin","ops"]`, vals)

	// realName 空白 → 回退 userName；无角色 payload → Roles 为 nil（非报错）
	token2, err := sm.Login(ctx, SessionPayload{UserID: 43, UserName: "fallback-user", RealName: "  "})
	require.NoError(t, err)
	id2, err := sm.Resolve(ctx, token2)
	require.NoError(t, err)
	assert.Equal(t, "fallback-user", id2.Name, "realName 空白必须回退 userName")
	assert.Empty(t, id2.Roles, "无角色会话 Resolve 的 Roles 必须为空而非报错")

	// 未知 token → ErrSessionNotFound（401 语义）
	_, err = sm.Resolve(ctx, "deadbeef")
	require.ErrorIs(t, err, ErrSessionNotFound)
}

// TestLogoutInvalidatesSession 登出即刻失效：Resolve → Logout → Resolve
// ErrSessionNotFound；索引 SRem；重复登出幂等返回 nil。
func TestLogoutInvalidatesSession(t *testing.T) {
	sm, mr, _ := newTestManager(t)
	ctx := context.Background()

	token, err := sm.Login(ctx, SessionPayload{UserID: 7, UserName: "u7", RealName: "用户七"})
	require.NoError(t, err)

	require.NoError(t, sm.Logout(ctx, token))
	_, err = sm.Resolve(ctx, token)
	require.ErrorIs(t, err, ErrSessionNotFound, "登出后原 token 必须立即失效")
	// 索引已 SRem
	n, err := mr.SCard(keyUserTokensPrefix + "7")
	require.NoError(t, err)
	assert.Zero(t, n, "登出后 user-tokens 索引必须清掉该 token")

	// 重复登出幂等：会话已不存在同样返回 nil
	require.NoError(t, sm.Logout(ctx, token), "重复登出必须幂等")
}

// TestSessionTTLRenewReset 活跃续期重置证明：Login(timeout=T) →
// FastForward(T-1min) → Resolve（命中并续期）→ 再 FastForward(T-1min) → Resolve 仍命中
// ——若 Resolve 未把 TTL 重置为完整 T，第二次推进后必过期。未续期对照：Login 后
// FastForward(T+1s) → Resolve 返回 ErrSessionNotFound（证明 FastForward 真的会使
// TTL 到期，续期断言非空转）。
func TestSessionTTLRenewReset(t *testing.T) {
	sm, mr, _ := newTestManager(t)
	ctx := context.Background()
	const ttl = 30 * time.Minute

	token, err := sm.Login(ctx, SessionPayload{UserID: 1, UserName: "ttl-user"})
	require.NoError(t, err)

	// 消耗 T-1min：仅剩 1min
	mr.FastForward(ttl - time.Minute)
	_, err = sm.Resolve(ctx, token)
	require.NoError(t, err, "剩余 1min 时 Resolve 必须命中")

	// 再推进 T-1min：续期生效 → 满血 TTL 重新走掉 29min，仍剩 1min 命中；
	// 若未重置（剩余仅 1min 被推过）此刻必 ErrSessionNotFound
	mr.FastForward(ttl - time.Minute)
	_, err = sm.Resolve(ctx, token)
	require.NoError(t, err, "Resolve 必须把 TTL 重置为完整 timeout（活跃续期）")

	// 未续期对照：新会话直接推过 TTL → 过期（证明 FastForward 机制真实生效）
	token2, err := sm.Login(ctx, SessionPayload{UserID: 2, UserName: "ttl-expired"})
	require.NoError(t, err)
	mr.FastForward(ttl + time.Second)
	_, err = sm.Resolve(ctx, token2)
	require.ErrorIs(t, err, ErrSessionNotFound, "推过 TTL 后会话必须过期（对照组）")
}

// TestKickoutCleansIndexAndDeadTokens 踢下线：索引含 1 live + 1 手工死
// token 时 KickoutUser——live 会话 DEL、死 token 也被 SRem（惰性清理）、SMembers 终态空。
func TestKickoutCleansIndexAndDeadTokens(t *testing.T) {
	sm, mr, rdb := newTestManager(t)
	ctx := context.Background()

	token, err := sm.Login(ctx, SessionPayload{UserID: 9, UserName: "kick-user"})
	require.NoError(t, err)

	// 手工注入死 token（会话从未写入，模拟 TTL 过期残留）
	deadToken := "dead0000dead0000dead0000dead0000dead0000dead0000dead0000dead0000"
	_, err = mr.SAdd(keyUserTokensPrefix+"9", deadToken)
	require.NoError(t, err)

	require.NoError(t, sm.KickoutUser(ctx, 9))

	_, err = sm.Resolve(ctx, token)
	require.ErrorIs(t, err, ErrSessionNotFound, "live 会话必须被 DEL")
	// 经 go-redis 客户端断言（缺失键 = 空集语义；miniredis 直接 API 对不存在的
	// Set 键会报 ERR no such key——SRem 全清后整个键已被删除）
	members, err := rdb.SMembers(ctx, keyUserTokensPrefix+"9").Result()
	require.NoError(t, err)
	assert.Empty(t, members, "索引终态必须为空（live+死 token 均被 SRem）")

	// 无索引用户踢下线幂等（SMembers 空 → no-op）
	require.NoError(t, sm.KickoutUser(ctx, 99), "无会话用户踢下线必须幂等")
}

// TestConcurrentApiKeyFullLogin 并发竞态显式处置（apikey 登录边缘场景）：两个
// goroutine 并发走"完整登录+BindAPIKey"——各自发新 token、两个 token 各自 Resolve
// 均有效（互不污染）、索引含两 token、映射 SET 最后写者赢（指向其一且仍有效）。
// 无 panic、无 error、无死键残留。
func TestConcurrentApiKeyFullLogin(t *testing.T) {
	sm, _, rdb := newTestManager(t)
	ctx := context.Background()

	const n = 2
	tokens := make(chan string, n)
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() {
			token, err := sm.Login(ctx, SessionPayload{UserID: 21, UserName: "race-user", RealName: "竞态用户"})
			if err != nil {
				errs <- err
				return
			}
			if err := sm.BindAPIKey(ctx, "ak-race", token, 21); err != nil {
				errs <- err
				return
			}
			tokens <- token
		}()
	}
	got := make([]string, 0, n)
	for i := 0; i < n; i++ {
		select {
		case err := <-errs:
			require.NoError(t, err)
		case tk := <-tokens:
			got = append(got, tk)
		}
	}
	require.Len(t, got, 2, "两并发登录都必须成功")
	require.NotEqual(t, got[0], got[1], "登录必发新 token（不复用旧值）")

	// 两个 token 各自有效、互不污染
	for _, tk := range got {
		id, err := sm.Resolve(ctx, tk)
		require.NoError(t, err)
		assert.Equal(t, int64(21), id.UserID)
	}
	// user-tokens 索引含两 token
	members, err := rdb.SMembers(ctx, keyUserTokensPrefix+"21").Result()
	require.NoError(t, err)
	assert.ElementsMatch(t, got, members, "索引必须含两枚并发 token")

	// 映射 SET 最后写者赢：指向其中之一且仍存活（另一 token 仍独立有效——无害并存）
	final, ok := sm.ResolveAPIKey(ctx, "ak-race")
	require.True(t, ok, "映射最终必须命中其一")
	assert.Contains(t, got, final)
}

// TestKickoutAPIKeyRevokesSessionAndMapping KickoutAPIKey 撤销指定 key 已
// 签发会话——会话 DEL（Resolve 失效）、user-tokens 索引 SRem、映射 DEL（单原子块）；
// 未签发过会话的 key 幂等 nil；映射指向死 token 的残留场景仅剩映射清理。
func TestKickoutAPIKeyRevokesSessionAndMapping(t *testing.T) {
	sm, _, rdb := newTestManager(t)
	ctx := context.Background()

	token, err := sm.Login(ctx, SessionPayload{UserID: 31, UserName: "ak-kick"})
	require.NoError(t, err)
	require.NoError(t, sm.BindAPIKey(ctx, "ak-revoke", token, 31))

	// 撤销前：映射命中、会话存活
	_, ok := sm.ResolveAPIKey(ctx, "ak-revoke")
	require.True(t, ok)
	_, err = sm.Resolve(ctx, token)
	require.NoError(t, err)

	require.NoError(t, sm.KickoutAPIKey(ctx, "ak-revoke"))

	// 会话即刻失效（认证中间件 401 语义）
	_, err = sm.Resolve(ctx, token)
	require.ErrorIs(t, err, ErrSessionNotFound, "撤销后 key 的存量会话必须 DEL")
	// 索引已 SRem（经 go-redis 客户端断言，缺失键 = 空集语义，与 TestKickoutCleansIndexAndDeadTokens 一致）
	members, err := rdb.SMembers(ctx, keyUserTokensPrefix+"31").Result()
	require.NoError(t, err)
	assert.Empty(t, members, "user-tokens 索引必须 SRem 撤销的 token")
	// 映射已清（下次 apikey-login 走完整登录换新 token，不复用死映射）
	_, ok = sm.ResolveAPIKey(ctx, "ak-revoke")
	assert.False(t, ok, "撤销后映射必须 DEL")

	// 幂等：重复撤销返回 nil；从未签发过会话的 key 同样幂等 nil
	require.NoError(t, sm.KickoutAPIKey(ctx, "ak-revoke"), "重复撤销必须幂等")
	require.NoError(t, sm.KickoutAPIKey(ctx, "ak-never-issued"), "无映射 key 撤销必须幂等 nil")

	// 死 token 残留场景：映射指向不存在会话 → 撤销仅剩双键清理，不报错
	dead := "dead0000dead0000dead0000dead0000dead0000dead0000dead0000dead0000"
	require.NoError(t, sm.BindAPIKey(ctx, "ak-dead-mapping", dead, 31))
	require.NoError(t, sm.KickoutAPIKey(ctx, "ak-dead-mapping"))
	_, ok = sm.ResolveAPIKey(ctx, "ak-dead-mapping")
	assert.False(t, ok, "死 token 残留场景映射必须被清理")
}

// TestBindAPIKeyDualKeys 双键契约直证：BindAPIKey 后 apikey-token 键
// （TTL=timeout）与 apikey-user 反查键（无 TTL——键寿命与 sys_api_key 行同寿命）
// 同时存在且载荷正确；LookupAPIKeyUser 命中/miss/损坏键三分支（损坏视同无键）。
func TestBindAPIKeyDualKeys(t *testing.T) {
	sm, mr, rdb := newTestManager(t)
	ctx := context.Background()
	const ttl = 30 * time.Minute

	token, err := sm.Login(ctx, SessionPayload{UserID: 55, UserName: "dual-key"})
	require.NoError(t, err)
	require.NoError(t, sm.BindAPIKey(ctx, "ak-dual", token, 55))

	// 映射键存在且带 TTL；反查键存在且无 TTL（redis TTL 协议：-1 = 无过期）
	tokVal, err := rdb.Get(ctx, keyAPIKeyPrefix+"ak-dual").Result()
	require.NoError(t, err)
	assert.Equal(t, token, tokVal)
	ttlGot, err := rdb.TTL(ctx, keyAPIKeyPrefix+"ak-dual").Result()
	require.NoError(t, err)
	assert.Greater(t, ttlGot, time.Duration(0), "apikey-token 键必须带 TTL")
	uidVal, err := rdb.Get(ctx, keyAPIKeyUserPrefix+"ak-dual").Result()
	require.NoError(t, err)
	assert.Equal(t, "55", uidVal, "反查键必须存 userID 十进制串")
	ttlUser, err := rdb.TTL(ctx, keyAPIKeyUserPrefix+"ak-dual").Result()
	require.NoError(t, err)
	assert.Equal(t, time.Duration(-1), ttlUser, "反查键必须无 TTL（键寿命与 sys_api_key 行同寿命）")

	// 反查命中
	uid, ok := sm.LookupAPIKeyUser(ctx, "ak-dual")
	require.True(t, ok)
	assert.Equal(t, int64(55), uid)

	// miss：键不存在 → false 无副作用
	_, ok = sm.LookupAPIKeyUser(ctx, "ak-never-bound")
	assert.False(t, ok, "无反查键必须 ok=false")

	// 损坏键（非数字载荷）视同无键：ok=false（幂等语义，不做 panic/错误放大）
	require.NoError(t, rdb.Set(ctx, keyAPIKeyUserPrefix+"ak-corrupt", "not-a-number", 0).Err())
	_, ok = sm.LookupAPIKeyUser(ctx, "ak-corrupt")
	assert.False(t, ok, "损坏反查键必须视同无键 ok=false")

	// 无 TTL 契约对照：FastForward 推过 timeout 后映射键过期消失、反查键仍在
	//（缺口正解——过期窗口内恒可反查）
	mr.FastForward(ttl + time.Second)
	_, err = rdb.Get(ctx, keyAPIKeyPrefix+"ak-dual").Result()
	require.Error(t, err, "推过 timeout 后映射键必须过期")
	uid, ok = sm.LookupAPIKeyUser(ctx, "ak-dual")
	require.True(t, ok, "反查键无 TTL——映射过期后仍可反查")
	assert.Equal(t, int64(55), uid)
}

// TestKickoutAPIKeyCleansDualKeys 清理路径一：KickoutAPIKey（管理面停用/
// 删除的撤销链）后双键均消失——键寿命与 key 行同寿命（UpdateStatus/Delete 两调用
// 点零改动即得清理）。
func TestKickoutAPIKeyCleansDualKeys(t *testing.T) {
	sm, _, rdb := newTestManager(t)
	ctx := context.Background()

	token, err := sm.Login(ctx, SessionPayload{UserID: 66, UserName: "dual-clean"})
	require.NoError(t, err)
	require.NoError(t, sm.BindAPIKey(ctx, "ak-clean", token, 66))

	require.NoError(t, sm.KickoutAPIKey(ctx, "ak-clean"))
	_, err = rdb.Get(ctx, keyAPIKeyPrefix+"ak-clean").Result()
	assert.Error(t, err, "撤销后 apikey-token 键必须消失")
	_, err = rdb.Get(ctx, keyAPIKeyUserPrefix+"ak-clean").Result()
	assert.Error(t, err, "撤销后 apikey-user 反查键必须消失（同寿命双删）")
}

// TestUnbindAPIKeyCleansDualKeys 清理路径二：UnbindAPIKey（复用分支发现
// 关联用户禁用/已删）后双键均消失——复用分支清理路径同寿命语义；幂等 DEL no-op。
func TestUnbindAPIKeyCleansDualKeys(t *testing.T) {
	sm, _, rdb := newTestManager(t)
	ctx := context.Background()

	token, err := sm.Login(ctx, SessionPayload{UserID: 77, UserName: "dual-unbind"})
	require.NoError(t, err)
	require.NoError(t, sm.BindAPIKey(ctx, "ak-unbind", token, 77))

	require.NoError(t, sm.UnbindAPIKey(ctx, "ak-unbind"))
	_, err = rdb.Get(ctx, keyAPIKeyPrefix+"ak-unbind").Result()
	assert.Error(t, err, "解绑后 apikey-token 键必须消失")
	_, err = rdb.Get(ctx, keyAPIKeyUserPrefix+"ak-unbind").Result()
	assert.Error(t, err, "解绑后 apikey-user 反查键必须消失")

	// 幂等：重复解绑 no-op
	require.NoError(t, sm.UnbindAPIKey(ctx, "ak-unbind"), "重复解绑必须幂等")
}

// TestApiKeyBindingBranches apiKey→token 映射两分支：BindAPIKey 后 ResolveAPIKey
// 命中返回原 token；DEL 会话（死 token）后 ResolveAPIKey 返回 ok=false（双检兜底）。
func TestApiKeyBindingBranches(t *testing.T) {
	sm, mr, _ := newTestManager(t)
	ctx := context.Background()

	token, err := sm.Login(ctx, SessionPayload{UserID: 11, UserName: "ak-user"})
	require.NoError(t, err)
	require.NoError(t, sm.BindAPIKey(ctx, "ak-live", token, 11))

	got, ok := sm.ResolveAPIKey(ctx, "ak-live")
	require.True(t, ok, "映射命中且会话存活必须返回 token")
	assert.Equal(t, token, got)

	// 未绑定 key → ok=false
	_, ok = sm.ResolveAPIKey(ctx, "ak-never-bound")
	assert.False(t, ok, "未绑定映射必须 ok=false")

	// 死 token 兜底：DEL 会话后映射仍在（TTL 未到），但双检必须 ok=false
	mr.Del(keySessionPrefix + token)
	_, ok = sm.ResolveAPIKey(ctx, "ak-live")
	assert.False(t, ok, "指向死 token 的映射必须 ok=false（调用方走完整登录路径覆盖该键）")
}
