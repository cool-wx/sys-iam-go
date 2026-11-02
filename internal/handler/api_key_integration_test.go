//go:build integration

// API Key 域集成测试：CRUD 五端点 + 明文 key 32 hex 一次性
// 返回 + 仅 status 单列语义 + 停用/过期不可登录直证 + 六组路由无 token
// 401 遍历（白名单外无遗漏）——真实 MySQL+Redis 双容器。
package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hexKeyRe 32 位小写 hex（等价源 UUID 去连字符熵）。
var hexKeyRe = regexp.MustCompile(`^[0-9a-f]{32}$`)

// apiKeyEnvelope create/get 响应解包形状（无 omitempty 契约）。
type apiKeyEnvelope struct {
	Code int `json:"code"`
	Data struct {
		ID         string  `json:"id"`
		ApiKey     string  `json:"apiKey"`
		UserID     *string `json:"userId"`
		Username   *string `json:"username"`
		ExpireTime *string `json:"expireTime"`
		Status     *string `json:"status"`
		CreateTime string  `json:"createTime"`
		UpdateTime string  `json:"updateTime"`
		Remark     *string `json:"remark"`
	} `json:"data"`
}

// TestApiKeyCRUD CRUD 主链路：create 明文 key 32 hex；带不存在 userId
// 400「用户不存在: {id}」；带有效 userId username 冗余 = user.Username（源语义取
// username 非 realName）；PUT status 停用后其余列不动（仅 status 单列语义直证）；
// list 分页 envelope；delete 后 GET 404。另含六组路由 GET 无 token 401 遍历
// （白名单外无遗漏）。
func TestApiKeyCRUD(t *testing.T) {
	ts, _, adminToken, adminID := newTestApp(t)
	token := adminToken()

	// --- create（无 userId）→ 200 + 32 hex 明文 ---
	status, body := doAuthRequest(t, token, "POST", ts.URL+"/api/v1/api-keys", `{"remark":"无主 key"}`)
	require.Equal(t, http.StatusOK, status, "create 应 200: body=%s", body)
	var created apiKeyEnvelope
	require.NoError(t, json.Unmarshal(body, &created))
	require.NotEmpty(t, created.Data.ID)
	assert.Regexp(t, hexKeyRe, created.Data.ApiKey, "明文 key 必须为 32 位小写 hex: %s", created.Data.ApiKey)
	require.NotNil(t, created.Data.Status, "status 键必须存在（无 omitempty）")
	assert.Equal(t, "ENABLED", *created.Data.Status, "status 恒默认 ENABLED")
	// userId 键存在但值为 null（无 omitempty 契约：键缺失 ≠ null 值——Go 端指针为 nil）
	assert.Contains(t, string(body), `"userId":null`, "userId 键必须存在且为 null（无 omitempty 契约）")

	// --- create（带不存在 userId）→ 400 + 域化消息 ---
	status, body = doAuthRequest(t, token, "POST", ts.URL+"/api/v1/api-keys", `{"userId":"990001"}`)
	require.Equal(t, http.StatusBadRequest, status, "不存在 userId 应 400: body=%s", body)
	var nfEnv struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(body, &nfEnv))
	assert.Equal(t, 10400, nfEnv.Code)
	assert.Equal(t, "用户不存在: 990001", nfEnv.Message, "body=%s", body)

	// --- create（带有效 userId=admin）→ username 冗余 = admin 的 username ---
	adminIDStr := strconv.FormatInt(adminID, 10)
	status, body = doAuthRequest(t, token, "POST", ts.URL+"/api/v1/api-keys",
		`{"userId":"`+adminIDStr+`","expireTime":"2030-01-01 00:00:00","remark":"关联 key"}`)
	require.Equal(t, http.StatusOK, status, "带有效 userId 应 200: body=%s", body)
	var linked apiKeyEnvelope
	require.NoError(t, json.Unmarshal(body, &linked))
	require.NotNil(t, linked.Data.UserID)
	assert.Equal(t, adminIDStr, *linked.Data.UserID, "userId 应回显")
	require.NotNil(t, linked.Data.Username)
	assert.Equal(t, "admin", *linked.Data.Username, "username 冗余必须取 user.Username（源语义，非 realName）")
	require.NotNil(t, linked.Data.ExpireTime)
	assert.Contains(t, *linked.Data.ExpireTime, "2030-01-01", "expireTime 应按请求落库: body=%s", body)
	linkedKey := linked.Data.ApiKey

	// --- GET baseline → PUT status 停用 → GET 对比仅 status 变化 ---
	status, body = doAuthRequest(t, token, "GET", ts.URL+"/api/v1/api-keys/"+linked.Data.ID, "")
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	var before apiKeyEnvelope
	require.NoError(t, json.Unmarshal(body, &before))

	status, body = doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/api-keys/"+linked.Data.ID+"/status", `{"status":"DISABLED"}`)
	require.Equal(t, http.StatusOK, status, "停用应 200: body=%s", body)

	status, body = doAuthRequest(t, token, "GET", ts.URL+"/api/v1/api-keys/"+linked.Data.ID, "")
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	var after apiKeyEnvelope
	require.NoError(t, json.Unmarshal(body, &after))

	assert.Equal(t, "DISABLED", deref(after.Data.Status), "status 应已停用")
	assert.Equal(t, before.Data.ApiKey, after.Data.ApiKey, "api_key 不得被写路径覆写")
	assert.Equal(t, before.Data.Username, after.Data.Username, "username 不得变化")
	assert.Equal(t, before.Data.UserID, after.Data.UserID, "user_id 不得变化")
	assert.Equal(t, before.Data.ExpireTime, after.Data.ExpireTime, "expire_time 不得变化")
	assert.Equal(t, before.Data.CreateTime, after.Data.CreateTime, "create_time 不得变化")
	assert.Equal(t, before.Data.Remark, after.Data.Remark, "remark 不得变化（editApiKey 仅 status 单列）")

	// --- list 分页 envelope ---
	for i := 0; i < 2; i++ {
		status, body = doAuthRequest(t, token, "POST", ts.URL+"/api/v1/api-keys", `{}`)
		require.Equal(t, http.StatusOK, status, "body=%s", body)
	}
	status, body = doAuthRequest(t, token, "GET", ts.URL+"/api/v1/api-keys?page=1&size=2", "")
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	var listEnv struct {
		Code int `json:"code"`
		Data struct {
			Items []apiKeyEnvelope_Data `json:"items"`
			Total int                   `json:"total"`
			Page  int                   `json:"page"`
			Size  int                   `json:"size"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &listEnv))
	assert.Equal(t, 4, listEnv.Data.Total, "total 应为 4（1 无主+1 关联+2 循环；独立容器无存量 key）")
	require.Len(t, listEnv.Data.Items, 2, "size=2 分页")
	assert.Equal(t, 1, listEnv.Data.Page)
	assert.Equal(t, 2, listEnv.Data.Size)
	for _, it := range listEnv.Data.Items {
		assert.Regexp(t, hexKeyRe, it.ApiKey, "管理面 list 亦可见明文（源语义）")
	}

	// --- delete 后 GET 404 ---
	status, body = doAuthRequest(t, token, "DELETE", ts.URL+"/api/v1/api-keys/"+linked.Data.ID, "")
	require.Equal(t, http.StatusOK, status, "delete 应 200: body=%s", body)
	status, body = doAuthRequest(t, token, "GET", ts.URL+"/api/v1/api-keys/"+linked.Data.ID, "")
	require.Equal(t, http.StatusNotFound, status, "删除后 GET 应 404: body=%s", body)
	var delEnv struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(body, &delEnv))
	assert.Equal(t, 10300, delEnv.Code)
	assert.Equal(t, "API Key 不存在: "+linked.Data.ID, delEnv.Message, "body=%s", body)

	_ = linkedKey // 明文已在断言中使用；不落任何日志行

	// --- 六组路由 GET 无 token 401 遍历（白名单外无遗漏）---
	for _, path := range []string{
		"/api/v1/users",
		"/api/v1/users/123",
		"/api/v1/roles",
		"/api/v1/roles/123",
		"/api/v1/menus",
		"/api/v1/menus/123",
		"/api/v1/menus/tree",
		"/api/v1/depts",
		"/api/v1/depts/123",
		"/api/v1/depts/tree",
		"/api/v1/api-keys",
		"/api/v1/api-keys/123",
	} {
		status, body = doRequest(t, "GET", ts.URL+path, "")
		require.Equal(t, http.StatusUnauthorized, status, "GET %s 无 token 必须 401: body=%s", path, body)
	}
	// 写路径抽查（非 GET 同样受保护）
	status, body = doRequest(t, "PUT", ts.URL+"/api/v1/api-keys/123/status", `{"status":"DISABLED"}`)
	require.Equal(t, http.StatusUnauthorized, status, "PUT status 无 token 必须 401: body=%s", body)
	status, body = doRequest(t, "DELETE", ts.URL+"/api/v1/api-keys/123", "")
	require.Equal(t, http.StatusUnauthorized, status, "DELETE 无 token 必须 401: body=%s", body)
}

// TestApiKeyDisabledCannotLogin 停用立即生效直证：经管理面
// create 的 key 先 apikey-login 200 → PUT status 停用 → 同 key apikey-login
// 401/10503（FindByKey 登录查询恒带 status=ENABLED）。
func TestApiKeyDisabledCannotLogin(t *testing.T) {
	ts, _, adminToken, adminID := newTestApp(t)
	token := adminToken()

	// create 带有效 userId（否则 apikey-login 10504 未关联用户——登录链语义）
	status, body := doAuthRequest(t, token, "POST", ts.URL+"/api/v1/api-keys",
		`{"userId":"`+strconv.FormatInt(adminID, 10)+`"}`)
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	var created apiKeyEnvelope
	require.NoError(t, json.Unmarshal(body, &created))
	key := created.Data.ApiKey

	// 停用前 apikey-login 200
	status, body = apikeyLogin(t, ts, key)
	require.Equal(t, http.StatusOK, status, "停用前有效 key 登录应 200: body=%s", body)

	// 管理面停用
	status, body = doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/api-keys/"+created.Data.ID+"/status", `{"status":"DISABLED"}`)
	require.Equal(t, http.StatusOK, status, "body=%s", body)

	// 停用后同 key 登录 401/10503
	status, body = apikeyLogin(t, ts, key)
	require.Equal(t, http.StatusUnauthorized, status, "停用 key 登录必须 401: body=%s", body)
	var envelope struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(body, &envelope))
	assert.Equal(t, 10503, envelope.Code, "停用 key 必须 10503: body=%s", body)

	// 重新启用 → 恢复登录（单列语义可逆）
	status, body = doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/api-keys/"+created.Data.ID+"/status", `{"status":"ENABLED"}`)
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	status, body = apikeyLogin(t, ts, key)
	assert.Equal(t, http.StatusOK, status, "重新启用后必须可登录: body=%s", body)
}

// TestApiKeyRevokeKillsExistingSession 停用/删除 key 撤销已签发存量会话直证（
// 用户轴同族语义——认证中间件仅校验 Redis 会话存在性、不回查 key 状态，
// 停用只封 FindByKey 新登录不逐旧会话则被撤销凭据的 token 在整个滚动 TTL 内继续
// 完全可用）：api key 登录拿 tokenK → tokenK 可访问业务端点（会话存活基线）→ 停用
// key → tokenK 即刻 401（存量会话已撤，非仅登录封禁）；登录封禁语义不变（10503）；
// 重新启用后再登录走完整路径换新 token（死映射已被撤销清理，不得复用旧 token）→
// 删除路径同构直证：DELETE key → 新 token 即刻 401。
func TestApiKeyRevokeKillsExistingSession(t *testing.T) {
	ts, _, adminToken, adminID := newTestApp(t)
	token := adminToken()

	// create 关联 key + apikey-login → tokenK
	status, body := doAuthRequest(t, token, "POST", ts.URL+"/api/v1/api-keys",
		`{"userId":"`+strconv.FormatInt(adminID, 10)+`"}`)
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	var created apiKeyEnvelope
	require.NoError(t, json.Unmarshal(body, &created))
	key := created.Data.ApiKey

	status, body = apikeyLogin(t, ts, key)
	require.Equal(t, http.StatusOK, status, "apikey-login 应 200: body=%s", body)
	tokenK := extractToken(t, body)

	// 会话存活基线：tokenK 可访问业务端点（users 列表仅 Auth 中间件，无角色要求）
	status, body = doAuthRequest(t, tokenK, "GET", ts.URL+"/api/v1/users", "")
	require.Equal(t, http.StatusOK, status, "撤销前 key 会话必须可用: body=%s", body)

	// --- 停用轴：停用 key → 存量会话即刻撤销（主断言）---
	status, body = doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/api-keys/"+created.Data.ID+"/status", `{"status":"DISABLED"}`)
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	status, body = doAuthRequest(t, tokenK, "GET", ts.URL+"/api/v1/users", "")
	require.Equal(t, http.StatusUnauthorized, status, "停用 key 后旧 token 必须即刻 401（存量会话已撤）: body=%s", body)
	var unauthEnv struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(body, &unauthEnv))
	assert.Equal(t, 10100, unauthEnv.Code, "401/10100 未认证语义: body=%s", body)

	// 登录封禁语义不变（FindByKey 恒带 ENABLED 过滤）
	status, body = apikeyLogin(t, ts, key)
	require.Equal(t, http.StatusUnauthorized, status, "停用 key 登录必须 401: body=%s", body)
	var loginEnv struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(body, &loginEnv))
	assert.Equal(t, 10503, loginEnv.Code, "body=%s", body)

	// 重新启用 → 完整登录换新 token（映射已被撤销 DEL，不得复用旧 token）
	status, body = doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/api-keys/"+created.Data.ID+"/status", `{"status":"ENABLED"}`)
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	status, body = apikeyLogin(t, ts, key)
	require.Equal(t, http.StatusOK, status, "重新启用后必须可完整登录: body=%s", body)
	tokenK2 := extractToken(t, body)
	require.NotEqual(t, tokenK, tokenK2, "撤销清映射后必须走完整登录发新 token")
	status, body = doAuthRequest(t, tokenK2, "GET", ts.URL+"/api/v1/users", "")
	require.Equal(t, http.StatusOK, status, "新会话必须可用: body=%s", body)

	// --- 删除轴同构直证：DELETE key → 存量会话即刻撤销 ---
	status, body = doAuthRequest(t, token, "DELETE", ts.URL+"/api/v1/api-keys/"+created.Data.ID, "")
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	status, body = doAuthRequest(t, tokenK2, "GET", ts.URL+"/api/v1/users", "")
	require.Equal(t, http.StatusUnauthorized, status, "删除 key 后旧 token 必须即刻 401（Delete 同构）: body=%s", body)
}

// TestApiKeyExpiredCannotLogin 过期 key 不可登录：seed expire_time 为
// 过去时间的 key → apikey-login 401/10503（FindByKey 恒带 expire_time > NOW() 过滤）。
func TestApiKeyExpiredCannotLogin(t *testing.T) {
	ts, dsn, adminToken, adminID := newTestApp(t)
	_ = adminToken()

	past := "2020-01-01 00:00:00"
	seedApiKey(t, dsn, "ak-mgmt-expired", "admin", &adminID, &past, 1)
	status, body := apikeyLogin(t, ts, "ak-mgmt-expired")
	require.Equal(t, http.StatusUnauthorized, status, "过期 key 登录必须 401: body=%s", body)
	var envelope struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(body, &envelope))
	assert.Equal(t, 10503, envelope.Code, "过期 key 必须 10503: body=%s", body)
}

// apiKeyEnvelope_Data list items 元素形状（拆出以避免与顶层 envelope 混淆）。
type apiKeyEnvelope_Data struct {
	ID     string `json:"id"`
	ApiKey string `json:"apiKey"`
}

// openCheckRedis 打开测试专用 go-redis 连接（拨号 newTestApp 赋值的 testRedisAddr
// ——当前用例的 Redis 容器）+ cleanup 关闭。双键（iam:apikey-token:/
// iam:apikey-user:）存在性直查断言用。
func openCheckRedis(t *testing.T) *redis.Client {
	t.Helper()
	rdb := redis.NewClient(&redis.Options{Addr: testRedisAddr})
	t.Cleanup(func() { _ = rdb.Close() })
	return rdb
}

// TestApiKeyExpireRevocation 验收核心（过期 key 存量会话撤销）：
// 有效 key K 关联用户 U → apikey-login 成功拿 token T（双键已写）→ 直改 DB 置
// expire_time 为过去（模拟自然过期——无管理面触发点的缺口场景）→ 再次 apikey-login
// 401/10503 且**存量会话已被反查撤销**（T 请求 pri 端点 401——核心断言）→
// 第二次拒绝后反查键已 DEL（防反复触发）。幂等路径：从未登录过的过期 key 登录
// 10503 原样（无反查键无副作用）。停用分支（管理面撤销链端到端）：K2 停用经
// UpdateStatus → KickoutAPIKey → 旧 token 401 且双键均消失（redis.Nil 直查）。
func TestApiKeyExpireRevocation(t *testing.T) {
	ts, dsn, adminToken, _ := newTestApp(t)
	token := adminToken()
	rdb := openCheckRedis(t)

	// --- 过期轴：seed 有效 key K 关联用户 U → 完整登录（双键写入）---
	uid := seedUser(t, dsn, "expire-victim", testAdminHash, "过期受害者", 1, false)
	seedApiKey(t, dsn, "ak-expire-revoke", "expire-victim", &uid, nil, 1)
	status, body := apikeyLogin(t, ts, "ak-expire-revoke")
	require.Equal(t, http.StatusOK, status, "有效 key 登录应 200: body=%s", body)
	tokT := extractToken(t, body)
	// 完整登录后双键在（撤销链的前提）
	_, err := rdb.Get(ctx0(), "iam:apikey-user:ak-expire-revoke").Result()
	require.NoError(t, err, "完整登录后反查键必须已写: %v", err)

	// 直改 DB 过期（绕过管理面——自然过期无撤销触发点，恰是缺口场景）
	checkDB := openCheckDB(t, dsn)
	execMust(t, checkDB, "UPDATE sys_api_key SET expire_time = ? WHERE api_key = ?",
		"2020-01-01 00:00:00", "ak-expire-revoke")

	// 再次 apikey-login → 401/10503
	status, body = apikeyLogin(t, ts, "ak-expire-revoke")
	require.Equal(t, http.StatusUnauthorized, status, "过期 key 登录必须 401: body=%s", body)
	var envelope struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(body, &envelope))
	assert.Equal(t, 10503, envelope.Code, "body=%s", body)

	// 核心断言：旧 token T 请求 pri 端点 → 401（存量会话已被反查撤销）
	status, body = doAuthRequest(t, tokT, "GET", ts.URL+"/api/v1/users/"+strconv.FormatInt(uid, 10), "")
	require.Equal(t, http.StatusUnauthorized, status, "过期 key 二次登录后旧 token 必须 401（存量会话已撤）: body=%s", body)
	var unauth struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(body, &unauth))
	assert.Equal(t, 10100, unauth.Code, "401/10100 未认证语义: body=%s", body)

	// 第二次拒绝后双键均已 DEL（反查键防反复触发 + 映射同寿命清理）
	_, err = rdb.Get(ctx0(), "iam:apikey-user:ak-expire-revoke").Result()
	require.ErrorIs(t, err, redis.Nil, "撤销后反查键必须已 DEL")
	_, err = rdb.Get(ctx0(), "iam:apikey-token:ak-expire-revoke").Result()
	require.ErrorIs(t, err, redis.Nil, "撤销后映射键必须已 DEL")

	// 幂等路径：从未登录过的过期 key → 10503 原样，无反查键无副作用
	past := "2020-01-01 00:00:00"
	seedApiKey(t, dsn, "ak-expire-never-logged", "expire-victim", &uid, &past, 1)
	status, body = apikeyLogin(t, ts, "ak-expire-never-logged")
	require.Equal(t, http.StatusUnauthorized, status, "从未登录的过期 key 登录必须 10503: body=%s", body)
	var idemEnv struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(body, &idemEnv))
	assert.Equal(t, 10503, idemEnv.Code, "body=%s", body)
	_, err = rdb.Get(ctx0(), "iam:apikey-user:ak-expire-never-logged").Result()
	require.ErrorIs(t, err, redis.Nil, "从未完整登录的 key 必须无反查键（无副作用）")

	// --- 停用分支（管理面撤销链端到端，单测之外的集成级覆盖）：K2 关联 U2 ---
	uid2 := seedUser(t, dsn, "disable-victim", testAdminHash, "停用受害者", 1, false)
	seedApiKey(t, dsn, "ak-disable-revoke", "disable-victim", &uid2, nil, 1)
	status, body = apikeyLogin(t, ts, "ak-disable-revoke")
	require.Equal(t, http.StatusOK, status, "K2 有效 key 登录应 200: body=%s", body)
	tokT2 := extractToken(t, body)

	// admin 管理面停用（UpdateStatus → KickoutAPIKey 撤销链，双键清理零调用点改动）
	var keyRow struct {
		ID int64
	}
	require.NoError(t, checkDB.Raw("SELECT id FROM sys_api_key WHERE api_key = ?", "ak-disable-revoke").Scan(&keyRow).Error)
	status, body = doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/api-keys/"+strconv.FormatInt(keyRow.ID, 10)+"/status",
		`{"status":"DISABLED"}`)
	require.Equal(t, http.StatusOK, status, "管理面停用应 200: body=%s", body)

	// K2 旧 token 即刻 401 且双键均消失（redis.Nil 直查）
	status, body = doAuthRequest(t, tokT2, "GET", ts.URL+"/api/v1/users/"+strconv.FormatInt(uid2, 10), "")
	require.Equal(t, http.StatusUnauthorized, status, "停用后 K2 旧 token 必须 401: body=%s", body)
	_, err = rdb.Get(ctx0(), "iam:apikey-user:ak-disable-revoke").Result()
	require.ErrorIs(t, err, redis.Nil, "停用后 K2 反查键必须消失（KickoutAPIKey 双删）")
	_, err = rdb.Get(ctx0(), "iam:apikey-token:ak-disable-revoke").Result()
	require.ErrorIs(t, err, redis.Nil, "停用后 K2 映射键必须消失")
}

// ctx0 集成测试 Redis 直查共用 context（每调用新建——无超时需求，容器生命周期由
// newTestApp cleanup 管理）。
func ctx0() context.Context { return context.Background() }

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
