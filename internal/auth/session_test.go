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

// TestSessionLoginResolve 登录→校验往返：token 为 64 hex 字符（256-bit 熵，T-2-02）；
// Resolve 返回正确 Identity——含 roles JSON 往返与 realName 非空白优先/空白回退
// userName（等价源 SaTokenConfig.java:120-137 语义）。
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
	assert.Equal(t, []string{"super_admin", "ops"}, id.Roles, "roles 快照必须经 Resolve 往返（WR-05 RequireRoles 授权判定依据）")

	// 会话 Hash 载荷四键齐备（D-10：userId/userName/realName/roles）+ roles JSON 往返
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

// TestLogoutInvalidatesSession 登出即刻失效（AUTH-03）：Resolve → Logout → Resolve
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

// TestSessionTTLRenewReset 活跃续期重置证明（AUTH-05）：Login(timeout=T) →
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
	require.NoError(t, err, "Resolve 必须把 TTL 重置为完整 timeout（活跃续期，AUTH-05）")

	// 未续期对照：新会话直接推过 TTL → 过期（证明 FastForward 机制真实生效）
	token2, err := sm.Login(ctx, SessionPayload{UserID: 2, UserName: "ttl-expired"})
	require.NoError(t, err)
	mr.FastForward(ttl + time.Second)
	_, err = sm.Resolve(ctx, token2)
	require.ErrorIs(t, err, ErrSessionNotFound, "推过 TTL 后会话必须过期（对照组）")
}

// TestKickoutCleansIndexAndDeadTokens 踢下线（AUTH-04）：索引含 1 live + 1 手工死
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

// TestConcurrentApiKeyFullLogin 并发竞态显式处置（AUTH-02 边缘，D-03）：两个
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
			if err := sm.BindAPIKey(ctx, "ak-race", token); err != nil {
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
	require.NotEqual(t, got[0], got[1], "登录必发新 token（不复用旧值，T-2-03）")

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

// TestApiKeyBindingBranches apiKey→token 映射两分支：BindAPIKey 后 ResolveAPIKey
// 命中返回原 token；DEL 会话（死 token）后 ResolveAPIKey 返回 ok=false（双检兜底）。
func TestApiKeyBindingBranches(t *testing.T) {
	sm, mr, _ := newTestManager(t)
	ctx := context.Background()

	token, err := sm.Login(ctx, SessionPayload{UserID: 11, UserName: "ak-user"})
	require.NoError(t, err)
	require.NoError(t, sm.BindAPIKey(ctx, "ak-live", token))

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
