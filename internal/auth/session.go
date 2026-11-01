// SessionManager：Redis token-session 管理器（等价源 Sa-Token 服务端会话模式——
// 随机 token ↔ Redis 会话 Hash，非 JWT）。Redis key 约定为全项目契约（Phase 3+
// 依赖），本文件为唯一事实源。
package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// Redis key 前缀约定（全项目契约）：
//   - iam:session:{token}       Hash: userId/userName/realName/roles(JSON 数组串)，
//     TTL=timeout，每次 Resolve 命中重置为完整 timeout（滚动会话，AUTH-05）
//   - iam:user-tokens:{userId}  Set of token，无 TTL——多 token 各自寿命不同（D-08
//     多端并存），死 token 由 KickoutUser 惰性清理
//   - iam:apikey-token:{apiKey} String=token，TTL=timeout；apikey 轴撤销（WR-06：
//     管理面停用/删除 key）经 KickoutAPIKey 主动删映射连带会话；用户轴的登出/踢下线
//     仍不主动删——无 token→apiKey 反向索引，指向死 token 时由 ResolveAPIKey 的会话
//     Exists 双检兜底走完整登录并覆盖该键（TTL 窗口内无害残留，02-CONTEXT.md 认可）
const (
	keySessionPrefix    = "iam:session:"
	keyUserTokensPrefix = "iam:user-tokens:"
	keyAPIKeyPrefix     = "iam:apikey-token:"
)

// ErrSessionNotFound 会话不存在/已过期。包级哨兵错误——注意 go-redis 的 HGetAll
// 对不存在的 key 返回【空 map + nil error】而非 redis.Nil，故本管理器主动归一为
// 本错误；认证中间件以 errors.Is 区分 401（会话缺失）与 500（Redis 故障）。
var ErrSessionNotFound = errors.New("session not found")

// SessionPayload 登录会话载荷（D-10：userId/userName/realName/roles 四键——
// permissions 快照不放，Phase 3 Casbin 即时生效语义取代登录快照）。
type SessionPayload struct {
	UserID   int64
	UserName string
	RealName string
	Roles    []string
}

// SessionManager 会话管理器：登录发 token、校验续期、登出、踢下线、API Key 绑定。
type SessionManager struct {
	rdb     *redis.Client
	timeout time.Duration
}

// NewSessionManager 构造。timeout 为会话 TTL（auth.session_timeout）；每次 Resolve
// 命中重置为完整 timeout（滚动会话）。
func NewSessionManager(rdb *redis.Client, timeout time.Duration) *SessionManager {
	return &SessionManager{rdb: rdb, timeout: timeout}
}

// randToken 生成会话 token：crypto/rand 32 字节 → hex（64 hex 字符，256-bit 熵，
// T-2-02；仿 requestid.go 样板，禁 uuid）。登录必发新 token 不复用旧值（防会话
// 固定/重放，T-2-03）；token 仅经 Authorization Bearer 传递，不进 URL/query。
func randToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Login 登录：发新 token，在一个 MULTI/EXEC 原子块内写会话 Hash（TTL=timeout）+
// SADD 用户 token 反查索引（索引无 TTL，D-08）。写坏一半（如 SADD 失败）随 MULTI
// 整体回滚，不会出现"有会话无索引"的半态。
func (sm *SessionManager) Login(ctx context.Context, payload SessionPayload) (string, error) {
	token, err := randToken()
	if err != nil {
		return "", err
	}
	rolesJSON, err := json.Marshal(payload.Roles)
	if err != nil {
		return "", err
	}
	sessionKey := keySessionPrefix + token
	userTokensKey := keyUserTokensPrefix + strconv.FormatInt(payload.UserID, 10)
	if _, err := sm.rdb.TxPipelined(ctx, func(p redis.Pipeliner) error {
		p.HSet(ctx, sessionKey, map[string]any{
			"userId":   strconv.FormatInt(payload.UserID, 10),
			"userName": payload.UserName,
			"realName": payload.RealName,
			"roles":    string(rolesJSON),
		})
		p.Expire(ctx, sessionKey, sm.timeout)
		p.SAdd(ctx, userTokensKey, token)
		return nil
	}); err != nil {
		return "", err
	}
	return token, nil
}

// Resolve 校验 token 并活跃续期：命中即 EXPIRE 重置为完整 timeout（AUTH-05——两次
// 访问间隔只要小于 timeout，会话永不自然过期）；未命中返回 ErrSessionNotFound。
// Identity.Name 按 realName 非空白优先、回退 userName（等价源 SaTokenConfig.java:
// 120-137 审计操作人语义，D-16 锁定）。
func (sm *SessionManager) Resolve(ctx context.Context, token string) (Identity, error) {
	sessionKey := keySessionPrefix + token
	vals, err := sm.rdb.HGetAll(ctx, sessionKey).Result()
	if err != nil {
		// Redis 故障：原样上抛，由认证中间件 fail-closed 译 500（T-2-04）
		return Identity{}, err
	}
	if len(vals) == 0 {
		return Identity{}, ErrSessionNotFound
	}
	if err := sm.rdb.Expire(ctx, sessionKey, sm.timeout).Err(); err != nil {
		return Identity{}, err
	}
	uid, parseErr := strconv.ParseInt(vals["userId"], 10, 64)
	if parseErr != nil {
		// 会话载荷损坏（不可能由本管理器写入）：视同会话不存在
		return Identity{}, ErrSessionNotFound
	}
	name := strings.TrimSpace(vals["realName"])
	if name == "" {
		name = vals["userName"]
	}
	// roles 快照出会话（WR-05：RequireRoles 授权 gate 判定依据）。载荷损坏/缺失
	// 不视为错误——roles=nil 即无任何角色，授权 gate 自然拒绝，比 fail-closed 到
	// 500 更贴合"会话本体有效、角色为空"的语义。
	var roles []string
	if raw := vals["roles"]; raw != "" {
		if err := json.Unmarshal([]byte(raw), &roles); err != nil {
			roles = nil
		}
	}
	return Identity{UserID: uid, Name: name, Roles: roles}, nil
}

// Logout 登出：DEL 会话 + SREM 索引（单 MULTI/EXEC 原子块）。幂等——会话已不存在
// （重复登出/已被踢）同样返回 nil（AUTH-03：登出后原 token 必然失效）。
func (sm *SessionManager) Logout(ctx context.Context, token string) error {
	sessionKey := keySessionPrefix + token
	userIDStr, err := sm.rdb.HGet(ctx, sessionKey, "userId").Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return err
	}
	// 会话已不存在时 userIDStr 为空串：SRem 落在不存在的索引键上，Redis 语义为 no-op，
	// DEL 同样幂等——整链不产生错误。
	userTokensKey := keyUserTokensPrefix + userIDStr
	_, err = sm.rdb.TxPipelined(ctx, func(p redis.Pipeliner) error {
		p.Del(ctx, sessionKey)
		p.SRem(ctx, userTokensKey, token)
		return nil
	})
	return err
}

// KickoutUser 踢下线（AUTH-04）：SMembers 反查索引 → 逐 token Exists——live 会话
// DEL、索引内全部 SRem（死 token 惰性清理：Sa-Token 同结构亦有死键堆积，靠此收敛，
// 02-RESEARCH 研究重点 3 语义澄清）。
func (sm *SessionManager) KickoutUser(ctx context.Context, userID int64) error {
	userTokensKey := keyUserTokensPrefix + strconv.FormatInt(userID, 10)
	tokens, err := sm.rdb.SMembers(ctx, userTokensKey).Result()
	if err != nil {
		return err
	}
	for _, tk := range tokens {
		sessionKey := keySessionPrefix + tk
		live, err := sm.rdb.Exists(ctx, sessionKey).Result()
		if err != nil {
			return err
		}
		if live == 1 {
			if err := sm.rdb.Del(ctx, sessionKey).Err(); err != nil {
				return err
			}
		}
		if err := sm.rdb.SRem(ctx, userTokensKey, tk).Err(); err != nil {
			return err
		}
	}
	return nil
}

// BindAPIKey 绑定 apiKey→token 映射（TTL=timeout，与会话同寿命）。
func (sm *SessionManager) BindAPIKey(ctx context.Context, apiKey, token string) error {
	return sm.rdb.Set(ctx, keyAPIKeyPrefix+apiKey, token, sm.timeout).Err()
}

// UnbindAPIKey 解除 apiKey→token 映射（WR-02：复用分支发现关联用户被禁用/已删时
// 清理——映射指向的会话不再可信，不得留作禁用身份的续命通道）。幂等：键不存在
// DEL 为 no-op；Redis 故障原样上抛由调用方取舍（清映射失败最坏退化为旧行为——
// 下次复用分支再次拒绝并重试清理，无放大效应）。
func (sm *SessionManager) UnbindAPIKey(ctx context.Context, apiKey string) error {
	return sm.rdb.Del(ctx, keyAPIKeyPrefix+apiKey).Err()
}

// KickoutAPIKey 撤销指定 API Key 已签发的存量会话（WR-06 修复，CR-01/D-09 用户轴
// 同构语义在 apikey 轴的有意安全增强——源系统停用/删除 key 不踢 Sa-Token 会话，
// 此处偏离源行为，由 ApiKeyService 注释声明裁决依据）：经 iam:apikey-token:{apiKey}
// 映射反查 token → DEL 会话 + SREM 用户 token 索引 + DEL 映射（单 MULTI/EXEC 原子块，
// Logout+UnbindAPIKey 合体形状）。幂等——映射不存在（key 从未经 apikey-login 签发过
// 会话）返回 nil；映射指向死 token 的历史残留同样收敛（DEL/SREM 均 no-op，仅剩映射
// 清理，Logout 同构语义）。
//
// 已知边界（既有残留语义的忠实记录，非本方法缺陷）：映射仅指向【最新】token。
// apikey-login 复用分支（AUTH-02）保证映射存活期内同一 key 只有一个活跃 token；映射
// TTL 过期后重新完整登录会覆盖旧映射，彼时若旧 token 因会话滚动续期仍存活，则无从
// 反查（无 token→apiKey 反向索引，02-CONTEXT.md 认可）——该窗口内旧 token 不在撤销
// 覆盖范围，至其自然 TTL 到期。
func (sm *SessionManager) KickoutAPIKey(ctx context.Context, apiKey string) error {
	mappingKey := keyAPIKeyPrefix + apiKey
	token, err := sm.rdb.Get(ctx, mappingKey).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil // 无映射 = 该 key 无已签发会话可撤
		}
		return err
	}
	sessionKey := keySessionPrefix + token
	userIDStr, err := sm.rdb.HGet(ctx, sessionKey, "userId").Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return err
	}
	// 会话已不存在（映射指向死 token）时 userIDStr 为空串：SRem 落在不存在的索引键上
	// 为 no-op、DEL 幂等——仅剩映射清理生效，整链不产生错误。
	userTokensKey := keyUserTokensPrefix + userIDStr
	_, err = sm.rdb.TxPipelined(ctx, func(p redis.Pipeliner) error {
		p.Del(ctx, sessionKey)
		p.SRem(ctx, userTokensKey, token)
		p.Del(ctx, mappingKey)
		return nil
	})
	return err
}

// ResolveAPIKey apiKey→token 解析：映射命中且指向会话仍存活才返回 token（双检）；
// 映射缺失或指向死 token 均返回 ok=false——调用方走完整登录路径并覆盖该键。
func (sm *SessionManager) ResolveAPIKey(ctx context.Context, apiKey string) (string, bool) {
	token, err := sm.rdb.Get(ctx, keyAPIKeyPrefix+apiKey).Result()
	if err != nil {
		// redis.Nil（映射不存在）或 Redis 故障——后者由后续完整登录路径 fail-closed 暴露
		return "", false
	}
	live, err := sm.rdb.Exists(ctx, keySessionPrefix+token).Result()
	if err != nil || live != 1 {
		return "", false
	}
	return token, true
}
