//go:build integration

// 认证链路集成测试（真实 MySQL + Redis 双容器）。
// 覆盖：存量 Hutool BCrypt $2a$ 哈希零迁移登录（兼容性活体直证）、登录→访问→
// 登出→401 端到端、白名单四路径收紧、防枚举两路径逐字节相同、禁用
// 10502、畸形 hash 401 非 500。
package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cool-wx/sys-iam-go/pkg/ids"
)

// 本库 admin 用户真实存量 Hutool BCrypt 哈希（明文 admin123）——Go x/crypto/bcrypt
// 兼容性已在研究阶段实测（02-RESEARCH：三源活体代证），此处作为 fixture 直证零迁移。
const (
	testAdminHash     = "$2a$10$7JB720yubVSZvUI0rEqK/.VqGOZTH.ulu33dHOiBE8ByOhJIrdAu2"
	testAdminPass     = "admin123"
	testAdminName     = "admin"
	testAdminRealName = "管理员"
)

// seedUser 直插 sys_user 行（create 端点要求认证，测试种子必须自举；
// 密码哈希/status/builtin/realName 可配——畸形 hash fixture 由此注入）。
// 返回雪花 id（审计断言需要）。
func seedUser(t *testing.T, dsn, username, passwordHash, realName string, status int8, builtin bool) int64 {
	t.Helper()
	db := openCheckDB(t, dsn)
	id := int64(ids.NewID())
	rows := execMust(t, db,
		"INSERT INTO sys_user (id, username, password, real_name, status, builtin, create_time, update_time) VALUES (?, ?, ?, ?, ?, ?, NOW(), NOW())",
		id, username, passwordHash, realName, status, builtin,
	)
	require.EqualValues(t, 1, rows, "seedUser 必须插入 1 行")
	return id
}

// seedApiKey 直插 sys_api_key 行（apikey-login 测试种子自举；userID/expireTime
// nil → NULL 列——未关联用户/永不过期两种源语义）。
func seedApiKey(t *testing.T, dsn, apiKey, username string, userID *int64, expireTime *string, status int8) int64 {
	t.Helper()
	db := openCheckDB(t, dsn)
	id := int64(ids.NewID())
	rows := execMust(t, db,
		"INSERT INTO sys_api_key (id, api_key, username, user_id, expire_time, status, create_time, update_time) VALUES (?, ?, ?, ?, ?, ?, NOW(), NOW())",
		id, apiKey, username, userID, expireTime, status,
	)
	require.EqualValues(t, 1, rows, "seedApiKey 必须插入 1 行")
	return id
}

// apikeyLogin 以 apiKey 调 apikey-login 端点，返回 (status, body)。
func apikeyLogin(t *testing.T, ts *httptest.Server, apiKey string) (int, []byte) {
	t.Helper()
	return doRequest(t, "POST", ts.URL+"/api/v1/auth/apikey-login", `{"apiKey":"`+apiKey+`"}`)
}

// seedRole 直插 sys_role 行，返回 roleId（kickout 授权测试种子：角色在
// 登录时装入会话 roles 快照）。data_scope 不写（NULL 列）——档位驱动的
// 行过滤测试用 seedRoleWithScope 变体。
func seedRole(t *testing.T, dsn, roleName, roleCode string, builtin bool) int64 {
	t.Helper()
	db := openCheckDB(t, dsn)
	id := int64(ids.NewID())
	rows := execMust(t, db,
		"INSERT INTO sys_role (id, role_name, role_code, status, builtin, create_time, update_time) VALUES (?, ?, ?, 1, ?, NOW(), NOW())",
		id, roleName, roleCode, builtin,
	)
	require.EqualValues(t, 1, rows, "seedRole 必须插入 1 行")
	return id
}

// seedRoleWithScope seedRole 的档位变体（data_scope 直插——档位驱动的
// 行过滤断言需要非 NULL 档位行）。
func seedRoleWithScope(t *testing.T, dsn, roleName, roleCode string, builtin bool, dataScope int8) int64 {
	t.Helper()
	db := openCheckDB(t, dsn)
	id := int64(ids.NewID())
	rows := execMust(t, db,
		"INSERT INTO sys_role (id, role_name, role_code, data_scope, status, builtin, create_time, update_time) VALUES (?, ?, ?, ?, 1, ?, NOW(), NOW())",
		id, roleName, roleCode, dataScope, builtin,
	)
	require.EqualValues(t, 1, rows, "seedRoleWithScope 必须插入 1 行")
	return id
}

// seedUserWithDeptCreator seedUser 的数据面变体（五档行集断言）：dept_id
// 与 create_by_id 直插——档 2/3/4 按 dept_id 过滤、档 1 按 create_by_id 过滤的
// 行集数据源。deptID/creatorID 可 nil（NULL 列）。
func seedUserWithDeptCreator(t *testing.T, dsn, username, passwordHash, realName string, status int8, builtin bool, deptID *int64, creatorID *int64) int64 {
	t.Helper()
	db := openCheckDB(t, dsn)
	id := int64(ids.NewID())
	rows := execMust(t, db,
		"INSERT INTO sys_user (id, username, password, real_name, dept_id, create_by_id, status, builtin, create_time, update_time) VALUES (?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW())",
		id, username, passwordHash, realName, deptID, creatorID, status, builtin,
	)
	require.EqualValues(t, 1, rows, "seedUserWithDeptCreator 必须插入 1 行")
	return id
}

// attachRoleToUser 直插 sys_user_role 关联行（username 列测试种子不较真——源命名
// 错位语义与登录快照无关）。必须在被测用户登录【之前】调用——roles 是登录时快照。
func attachRoleToUser(t *testing.T, dsn string, userID, roleID int64, roleName string) {
	t.Helper()
	db := openCheckDB(t, dsn)
	rows := execMust(t, db,
		"INSERT INTO sys_user_role (id, user_id, username, role_id, role_name, create_time, update_time) VALUES (?, ?, '', ?, ?, NOW(), NOW())",
		int64(ids.NewID()), userID, roleID, roleName,
	)
	require.EqualValues(t, 1, rows, "attachRoleToUser 必须插入 1 行")
}

// extractToken 从登录响应 envelope 提取 token。
func extractToken(t *testing.T, body []byte) string {
	t.Helper()
	var envelope struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &envelope), "body=%s", body)
	return envelope.Data.Token
}

// TestApiKeyLoginFlow API Key 登录主链路：有效 key 登录拿 tokenA →
// 同 key 再登复用同一 token（tokenB==tokenA）；停用 key 10503；过期 key 10503；
// 未关联用户 key 10504。
func TestApiKeyLoginFlow(t *testing.T) {
	ts, dsn, adminToken, adminID := newTestApp(t)
	_ = adminToken()

	// 有效 key → 200 拿 tokenA
	seedApiKey(t, dsn, "ak-valid-key", testAdminName, &adminID, nil, 1)
	status, body := apikeyLogin(t, ts, "ak-valid-key")
	require.Equal(t, http.StatusOK, status, "有效 key 登录应 200: body=%s", body)
	tokenA := extractToken(t, body)
	require.NotEmpty(t, tokenA)

	// 同 key 二次登录 → 复用同一 token（映射命中且 session 存活）
	status, body = apikeyLogin(t, ts, "ak-valid-key")
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	tokenB := extractToken(t, body)
	assert.Equal(t, tokenA, tokenB, "同一 API Key 二次登录必须复用同一 token")

	// 停用 key → 10503（停用立即生效）
	seedApiKey(t, dsn, "ak-disabled-key", testAdminName, &adminID, nil, 0)
	status, body = apikeyLogin(t, ts, "ak-disabled-key")
	require.Equal(t, http.StatusUnauthorized, status, "body=%s", body)
	var envelope struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(body, &envelope))
	assert.Equal(t, 10503, envelope.Code, "停用 key 必须 10503: body=%s", body)

	// 过期 key → 10503
	past := "2020-01-01 00:00:00"
	seedApiKey(t, dsn, "ak-expired-key", testAdminName, &adminID, &past, 1)
	status, body = apikeyLogin(t, ts, "ak-expired-key")
	require.Equal(t, http.StatusUnauthorized, status, "body=%s", body)
	var expiredEnvelope struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(body, &expiredEnvelope))
	assert.Equal(t, 10503, expiredEnvelope.Code, "过期 key 必须 10503: body=%s", body)

	// 未知 key → 10503
	status, body = apikeyLogin(t, ts, "ak-never-seeded")
	require.Equal(t, http.StatusUnauthorized, status, "body=%s", body)
	var unknownEnvelope struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(body, &unknownEnvelope))
	assert.Equal(t, 10503, unknownEnvelope.Code)

	// user_id NULL key → 10504（源"API Key 未关联用户"）
	seedApiKey(t, dsn, "ak-orphan-key", "orphan-owner", nil, nil, 1)
	status, body = apikeyLogin(t, ts, "ak-orphan-key")
	require.Equal(t, http.StatusUnauthorized, status, "body=%s", body)
	var orphanEnvelope struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(body, &orphanEnvelope))
	assert.Equal(t, 10504, orphanEnvelope.Code, "未关联用户 key 必须 10504: body=%s", body)
}

// TestApikeyLoginReuseRejectsDisabledUser 复用分支状态校验（回归）：有效 key
// 登录建立映射 → 库内直改禁用关联用户（绕过 UpdateStatus 的 kick——模拟踢失败场景
// "禁用已落库但 kick 未发生"窗口/外部禁用）→ 同 key 再 apikey-login 必须拒绝
// 10506（不得复用确认）；随后库内恢复启用（token 仍存活）→ 再次登录必须发新
// token——直证禁用拒绝时已清旧映射（若未清，此处会原样复用旧 token）。
func TestApikeyLoginReuseRejectsDisabledUser(t *testing.T) {
	ts, dsn, adminToken, _ := newTestApp(t)
	_ = adminToken()

	uid := seedUser(t, dsn, "apikey-victim", testAdminHash, "受害者", 1, false)
	seedApiKey(t, dsn, "ak-disabled-reuse", "apikey-victim", &uid, nil, 1)

	// 首次完整登录：映射建立
	status, body := apikeyLogin(t, ts, "ak-disabled-reuse")
	require.Equal(t, http.StatusOK, status, "首次登录应 200: body=%s", body)
	tokenA := extractToken(t, body)
	require.NotEmpty(t, tokenA)

	// 库内直改禁用（不触发 kick——tokenA 会话仍存活）
	checkDB := openCheckDB(t, dsn)
	execMust(t, checkDB, "UPDATE sys_user SET status = 0 WHERE id = ?", uid)

	// 复用分支必须校验状态：拒绝复用，顺延完整路径 10506
	status, body = apikeyLogin(t, ts, "ak-disabled-reuse")
	require.Equal(t, http.StatusUnauthorized, status, "禁用用户的复用登录必须 401: body=%s", body)
	var envelope struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(body, &envelope))
	assert.Equal(t, 10506, envelope.Code, "禁用关联用户必须 10506: body=%s", body)

	// 库内恢复启用（tokenA 仍存活，未 kick）
	execMust(t, checkDB, "UPDATE sys_user SET status = 1 WHERE id = ?", uid)

	// 再次登录：旧映射已在禁用拒绝时被清——必须完整登录发新 token；
	// 若清映射缺失，此处会命中复用分支原样返回 tokenA（直证清理生效）
	status, body = apikeyLogin(t, ts, "ak-disabled-reuse")
	require.Equal(t, http.StatusOK, status, "恢复启用后应可登录: body=%s", body)
	tokenB := extractToken(t, body)
	assert.NotEqual(t, tokenA, tokenB, "旧映射必须已被清——不得复用禁用期间确认过的 tokenA")
}

// TestKickoutFlow 踢下线端到端：目标用户双端登录两 token →
// kickout {userId} → 两 token 访问均 401 → 该用户可重新登录（踢下线非封禁）。
// kickout 仅 super_admin 可调——操作者 admin 须先挂该角色再登录
// （roles 是登录时快照）。
func TestKickoutFlow(t *testing.T) {
	ts, _, adminToken, adminID := newTestApp(t)

	// newTestApp 已内建 super_admin 角色并 attach 到 admin（boot 对账
	// 种子三件套）——此处原 seedRole("super_admin") 会撞 uk_role_code 1062，删除

	tokenA := adminToken()
	tokenB := adminToken()
	require.NotEqual(t, tokenA, tokenB, "双端登录应得两枚独立 token")

	// 前置：两 token 均有效
	status, body := doAuthRequest(t, tokenA, "GET", ts.URL+"/api/v1/users", "")
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	status, body = doAuthRequest(t, tokenB, "GET", ts.URL+"/api/v1/users", "")
	require.Equal(t, http.StatusOK, status, "body=%s", body)

	// kickout：目标 = admin 自己（kickout 仅 super_admin 可调——本测试
	// 操作者已持该角色快照）
	kickBody := `{"userId":"` + strconv.FormatInt(adminID, 10) + `"}`
	status, body = doAuthRequest(t, tokenB, "POST", ts.URL+"/api/v1/auth/kickout", kickBody)
	require.Equal(t, http.StatusOK, status, "kickout 应 200: body=%s", body)

	// 两 token 全部失效
	status, body = doAuthRequest(t, tokenA, "GET", ts.URL+"/api/v1/users", "")
	require.Equal(t, http.StatusUnauthorized, status, "tokenA 必须 401: body=%s", body)
	status, body = doAuthRequest(t, tokenB, "GET", ts.URL+"/api/v1/users", "")
	require.Equal(t, http.StatusUnauthorized, status, "tokenB 必须 401: body=%s", body)

	// 重新登录成功（踢下线非封禁）
	status, body = doRequest(t, "POST", ts.URL+"/api/v1/auth/login",
		`{"username":"`+testAdminName+`","password":"`+testAdminPass+`"}`)
	require.Equal(t, http.StatusOK, status, "被踢用户必须可重新登录: body=%s", body)
}

// TestKickoutRequiresSuperAdmin kickout 授权收紧（过渡收口回归）：
// 无角色普通用户 token 调 kickout → 403/10200（不再"任何有效 token 可踢任意
// 会话"——此前任意低权用户可踢掉 admin 全部会话）；super_admin token → 200。
func TestKickoutRequiresSuperAdmin(t *testing.T) {
	ts, dsn, adminToken, adminID := newTestApp(t)

	// newTestApp 已内建 super_admin 角色并 attach 到 admin——原
	// seedRole("super_admin") 会撞 uk_role_code 1062，删除

	// 低权用户（无任何角色）登录
	seedUser(t, dsn, "low-user", testAdminHash, "低权者", 1, false)
	lowToken := loginAs(t, ts, "low-user", testAdminPass)

	kickBody := `{"userId":"` + strconv.FormatInt(adminID, 10) + `"}`
	status, body := doAuthRequest(t, lowToken, "POST", ts.URL+"/api/v1/auth/kickout", kickBody)
	require.Equal(t, http.StatusForbidden, status, "无角色用户 kickout 必须 403: body=%s", body)
	var envelope struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(body, &envelope))
	assert.Equal(t, 10200, envelope.Code, "body=%s", body)
	assert.Equal(t, "无权限", envelope.Message)

	// 403 不得有副作用：admin 会话仍有效
	tk := adminToken()
	status, body = doAuthRequest(t, tk, "GET", ts.URL+"/api/v1/users", "")
	require.Equal(t, http.StatusOK, status, "403 拒绝不得影响目标会话: body=%s", body)

	// super_admin → 200（踢 admin 自己）
	status, body = doAuthRequest(t, tk, "POST", ts.URL+"/api/v1/auth/kickout", kickBody)
	require.Equal(t, http.StatusOK, status, "super_admin kickout 必须 200: body=%s", body)
}

// TestAuthFlow 登录→带 token 访问→登出→401 端到端（认证主链路）。
func TestAuthFlow(t *testing.T) {
	ts, _, adminToken, _ := newTestApp(t)
	token := adminToken()

	// 带 token 访问受保护路由 → 200
	status, body := doAuthRequest(t, token, "GET", ts.URL+"/api/v1/users", "")
	require.Equal(t, http.StatusOK, status, "body=%s", body)

	// 登出 → 200
	status, body = doAuthRequest(t, token, "POST", ts.URL+"/api/v1/auth/logout", "")
	require.Equal(t, http.StatusOK, status, "登出应 200: body=%s", body)

	// 同 token 再访问 → 401 + 10100（登出即刻失效）
	status, body = doAuthRequest(t, token, "GET", ts.URL+"/api/v1/users", "")
	require.Equal(t, http.StatusUnauthorized, status, "登出后原 token 必须 401: body=%s", body)
	var envelope struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(body, &envelope))
	assert.Equal(t, 10100, envelope.Code)
}

// TestAuthWhitelist 白名单收紧：login、apikey-login、healthz、
// readyz 四路径免 token 可达；其余路由无 token 一律 401。
func TestAuthWhitelist(t *testing.T) {
	ts, dsn, _, adminID := newTestApp(t)

	// 免 token 可达
	status, body := doRequest(t, "POST", ts.URL+"/api/v1/auth/login", `{"username":"`+testAdminName+`","password":"`+testAdminPass+`"}`)
	require.Equal(t, http.StatusOK, status, "login 白名单必须免 token 可达: body=%s", body)
	// 第四路径 apikey-login：无 token 且有效 key → 200（区别于"路由要认证"的 401/10100）
	seedApiKey(t, dsn, "ak-whitelist-key", testAdminName, &adminID, nil, 1)
	status, body = apikeyLogin(t, ts, "ak-whitelist-key")
	require.Equal(t, http.StatusOK, status, "apikey-login 白名单必须免 token 可达: body=%s", body)
	require.NotEmpty(t, extractToken(t, body))
	status, body = doRequest(t, "GET", ts.URL+"/healthz", "")
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	status, body = doRequest(t, "GET", ts.URL+"/readyz", "")
	require.Equal(t, http.StatusOK, status, "body=%s", body)

	// 非白名单：无 token → 401
	status, body = doRequest(t, "GET", ts.URL+"/api/v1/users", "")
	require.Equal(t, http.StatusUnauthorized, status, "受保护路由无 token 必须 401: body=%s", body)
	var envelope struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(body, &envelope))
	assert.Equal(t, 10100, envelope.Code)
}

// TestAuthLoginFailures 登录失败面（防枚举/防 500）：①用户不存在与密码错误两路径
// body 逐字节相同（防枚举）②禁用用户 10502 ③畸形 hash 401 非 500。
func TestAuthLoginFailures(t *testing.T) {
	ts, dsn, _, _ := newTestApp(t)

	// ① 用户不存在 vs 错误密码：逐字节相同
	status1, body1 := doRequest(t, "POST", ts.URL+"/api/v1/auth/login", `{"username":"ghost-user","password":"whatever"}`)
	status2, body2 := doRequest(t, "POST", ts.URL+"/api/v1/auth/login", `{"username":"`+testAdminName+`","password":"wrong-password"}`)
	require.Equal(t, http.StatusUnauthorized, status1)
	require.Equal(t, http.StatusUnauthorized, status2)
	assert.Equal(t, string(body1), string(body2), "防枚举：用户不存在与密码错误响应 body 必须逐字节相同")
	var envelope struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(body1, &envelope))
	assert.Equal(t, 10501, envelope.Code)
	assert.Equal(t, "用户名或密码错误", envelope.Message)

	// ② 禁用用户 → 独立错误码 10502（HTTP 401）
	seedUser(t, dsn, "disabled-user", testAdminHash, "被禁用者", 0, false)
	status, body := doRequest(t, "POST", ts.URL+"/api/v1/auth/login", `{"username":"disabled-user","password":"`+testAdminPass+`"}`)
	require.Equal(t, http.StatusUnauthorized, status, "body=%s", body)
	var disabledEnvelope struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(body, &disabledEnvelope))
	assert.Equal(t, 10502, disabledEnvelope.Code, "禁用用户必须 10502: body=%s", body)
	assert.Equal(t, "用户已被禁用", disabledEnvelope.Message)

	// ②（回归防护）禁用用户 + 错误密码 → 统一 10501（与"不存在/启用错密码"
	// 逐字节相同的语义）：状态校验在 bcrypt 成功之后——错密码不得提前返回 10502
	// 泄漏"存在且禁用"名单（防枚举）
	status, body = doRequest(t, "POST", ts.URL+"/api/v1/auth/login", `{"username":"disabled-user","password":"wrong-password"}`)
	require.Equal(t, http.StatusUnauthorized, status, "body=%s", body)
	var disabledWrongPw struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(body, &disabledWrongPw))
	assert.Equal(t, 10501, disabledWrongPw.Code, "禁用用户错密码必须统一 10501（不得泄漏禁用态）: body=%s", body)
	assert.Equal(t, "用户名或密码错误", disabledWrongPw.Message)
	assert.Equal(t, string(body1), string(body), "禁用错密码与不存在响应必须逐字节相同（防枚举）")

	// ③ 畸形 hash（10 字符垃圾值，本库 arm64 行同型 fixture）→ 10501 且 HTTP 401
	// 而非 500（任何 bcrypt error 一律译为登录失败）
	seedUser(t, dsn, "malformed-user", "Arm64@U", "畸形哈希者", 1, false)
	status, body = doRequest(t, "POST", ts.URL+"/api/v1/auth/login", `{"username":"malformed-user","password":"any-pass"}`)
	require.Equal(t, http.StatusUnauthorized, status, "畸形 hash 必须 401 而非 500: body=%s", body)
	var malformedEnvelope struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(body, &malformedEnvelope))
	assert.Equal(t, 10501, malformedEnvelope.Code, "畸形 hash 必须 10501（非 500/10001）: body=%s", body)
}
