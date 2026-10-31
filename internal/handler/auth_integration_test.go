//go:build integration

// 认证链路集成测试（真实 MySQL + Redis 双容器，D-10/D-12）。
// 覆盖：存量 Hutool BCrypt $2a$ 哈希零迁移登录（活体代证链 D-16）、登录→访问→
// 登出→401 端到端、白名单四路径收紧（AUTH-06）、防枚举两路径逐字节相同、禁用
// 10502、畸形 hash 401 非 500（T-2-08）。
package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cool-wx/sys-iam-go/pkg/ids"
)

// 本库 admin 用户真实存量 Hutool BCrypt 哈希（明文 admin123）——Go x/crypto/bcrypt
// 兼容性已在研究阶段实测（02-RESEARCH：三源活体代证），此处作为 fixture 直证零迁移。
const (
	testAdminHash    = "$2a$10$7JB720yubVSZvUI0rEqK/.VqGOZTH.ulu33dHOiBE8ByOhJIrdAu2"
	testAdminPass    = "admin123"
	testAdminName    = "admin"
	testAdminRealName = "管理员"
)

// seedUser 直插 sys_user 行（create 端点自 Phase 2 起要求认证，测试种子必须自举；
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

// TestAuthFlow 登录→带 token 访问→登出→401 端到端（AUTH-01/03/06 主链路）。
func TestAuthFlow(t *testing.T) {
	ts, _, adminToken, _ := newTestApp(t)
	token := adminToken()

	// 带 token 访问受保护路由 → 200
	status, body := doAuthRequest(t, token, "GET", ts.URL+"/api/v1/users", "")
	require.Equal(t, http.StatusOK, status, "body=%s", body)

	// 登出 → 200
	status, body = doAuthRequest(t, token, "POST", ts.URL+"/api/v1/auth/logout", "")
	require.Equal(t, http.StatusOK, status, "登出应 200: body=%s", body)

	// 同 token 再访问 → 401 + 10100（AUTH-03 即刻失效）
	status, body = doAuthRequest(t, token, "GET", ts.URL+"/api/v1/users", "")
	require.Equal(t, http.StatusUnauthorized, status, "登出后原 token 必须 401: body=%s", body)
	var envelope struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(body, &envelope))
	assert.Equal(t, 10100, envelope.Code)
}

// TestAuthWhitelist 白名单收紧（AUTH-06/D-15）：login（与 Task 2 的 apikey-login）、
// healthz、readyz 免 token 可达；其余路由无 token 一律 401。
func TestAuthWhitelist(t *testing.T) {
	ts, _, _, _ := newTestApp(t)

	// 免 token 可达
	status, body := doRequest(t, "POST", ts.URL+"/api/v1/auth/login", `{"username":"`+testAdminName+`","password":"`+testAdminPass+`"}`)
	require.Equal(t, http.StatusOK, status, "login 白名单必须免 token 可达: body=%s", body)
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

// TestAuthLoginFailures 登录失败面（T-2-01/T-2-08）：①用户不存在与密码错误两路径
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

	// ③ 畸形 hash（10 字符垃圾值，本库 arm64 行同型 fixture）→ 10501 且 HTTP 401
	// 而非 500（T-2-08：任何 bcrypt error 一律译为登录失败）
	seedUser(t, dsn, "malformed-user", "Arm64@U", "畸形哈希者", 1, false)
	status, body = doRequest(t, "POST", ts.URL+"/api/v1/auth/login", `{"username":"malformed-user","password":"any-pass"}`)
	require.Equal(t, http.StatusUnauthorized, status, "畸形 hash 必须 401 而非 500: body=%s", body)
	var malformedEnvelope struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(body, &malformedEnvelope))
	assert.Equal(t, 10501, malformedEnvelope.Code, "畸形 hash 必须 10501（非 500/10001）: body=%s", body)
}
