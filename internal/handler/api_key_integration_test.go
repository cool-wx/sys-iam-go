//go:build integration

// API Key 域集成测试（02-03 AKEY-01/02）：CRUD 五端点 + 明文 key 32 hex 一次性
// 返回 + 仅 status 单列语义 + 停用/过期不可登录直证（T-2-16）+ 六组路由无 token
// 401 遍历（AUTH-06 白名单外无遗漏）——真实 MySQL+Redis 双容器。
package handler_test

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"testing"

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

// TestApiKeyCRUD CRUD 主链路（AKEY-01）：create 明文 key 32 hex；带不存在 userId
// 400「用户不存在: {id}」；带有效 userId username 冗余 = user.Username（源语义取
// username 非 realName）；PUT status 停用后其余列不动（仅 status 单列语义直证）；
// list 分页 envelope；delete 后 GET 404。另含六组路由 GET 无 token 401 遍历
// （AUTH-06：白名单外无遗漏）。
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
	assert.Equal(t, before.Data.ApiKey, after.Data.ApiKey, "api_key 不得被写路径覆写（T-2-15）")
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

	_ = linkedKey // 明文已在断言中使用；不落任何日志行（T-2-15）

	// --- 六组路由 GET 无 token 401 遍历（AUTH-06 白名单外无遗漏）---
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

// TestApiKeyDisabledCannotLogin 停用立即生效直证（AKEY-02/T-2-16）：经管理面
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

// TestApiKeyExpiredCannotLogin 过期 key 不可登录（AKEY-02）：seed expire_time 为
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

// deref 安全解指针字符串。
func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
