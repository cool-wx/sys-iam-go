//go:build integration

// Dict/Config/Shift 三域集成测试（真实 MySQL+Redis 双容器，复用 newTestApp 基建，
// Phase 4 04-01）。覆盖：
//   - TestDictTypeCRUD：类型 CRUD 全链 + uk_dict_type 1062 翻译（400/10400
//     「字典类型已存在」）+ update 零值保护（DICT-01）
//   - TestDictDataWithTypeCheck：类型存在性前置校验（404/10300 源消息原文）+
//     dict_type_name 服务端冗余写（DICT-02）
//   - TestDictDataFilterByType：dict_type_id 可选过滤分支（DICT-03）
//   - TestConfigCRUDAndKey：config CRUD + key/{key} 字符串本体查询 + uk_config_key
//     1062 翻译（CONF-01，Task 2）
//   - TestShiftFullListNoPaging：全量数组直出无分页 envelope（SHFT-01 源行为直译，
//     Task 2）+ CRUD（SHFT-02）
//   - TestDictRoutePerm：无 token 401 / 无 perm 用户 403 / admin 200 三断言（D-04）
package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cool-wx/sys-iam-go/internal/model/po"
	"github.com/cool-wx/sys-iam-go/pkg/ids"
)

// int64ToStr 集成断言路径参数拼接用（雪花 ID → 十进制串）。
func int64ToStr(v int64) string {
	return strconv.FormatInt(v, 10)
}

// dictEnvelope 响应包装（与 pkg/resp.Envelope 同形；data 用 json.RawMessage 延迟解析）。
type dictEnvelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// dictTypeRespBody 字典类型响应体（集成断言所需子集；ID 用 ids.ID——响应序列化
// 为带引号字符串形态，UnmarshalJSON 双兼容）。
type dictTypeRespBody struct {
	ID       ids.ID  `json:"id"`
	DictName *string `json:"dictName"`
	DictType *string `json:"dictType"`
	Remark   *string `json:"remark"`
}

// dictDataRespBody 字典数据响应体（集成断言所需子集）。
type dictDataRespBody struct {
	ID           ids.ID  `json:"id"`
	DictTypeID   ids.ID  `json:"dictTypeId"`
	DictTypeName *string `json:"dictTypeName"`
	Label        *string `json:"label"`
	Value        *string `json:"value"`
}

// pageEnvelope 分页 envelope（items/total/page/size 齐——DICT-03 断言）。
type pageEnvelope struct {
	Items []json.RawMessage `json:"items"`
	Total int64             `json:"total"`
	Page  int               `json:"page"`
	Size  int               `json:"size"`
}

// createDictType 经 API 创建字典类型，返回 id（测试串联用）。
func createDictType(t *testing.T, ts *httptest.Server, token, name, typ string) int64 {
	t.Helper()
	body := `{"dictName":"` + name + `","dictType":"` + typ + `"}`
	status, respBody := doAuthRequest(t, token, "POST", ts.URL+"/api/v1/dict-types", body)
	require.Equal(t, http.StatusOK, status, "create dict-type 失败: body=%s", respBody)
	var env dictEnvelope
	require.NoError(t, json.Unmarshal(respBody, &env))
	var created dictTypeRespBody
	require.NoError(t, json.Unmarshal(env.Data, &created))
	return int64(created.ID)
}

// TestDictTypeCRUD 字典类型 CRUD 全链（DICT-01）：
// create→get→list→update 零值保护（不传字段不清空）→delete→get 404；
// dict_type 重复 create → 400 code 10400 消息「字典类型已存在」（uk_dict_type
// 1062 经 TranslateError 翻译，DICT-01 验收明文）；update 撞同 uk 同翻译
//（WR-02：白名单含唯一键列，Update 与 Create 同冲突同译，不透传 500）。
func TestDictTypeCRUD(t *testing.T) {
	ts, _, adminToken, _ := newTestApp(t)
	token := adminToken()

	// create
	status, body := doAuthRequest(t, token, "POST", ts.URL+"/api/v1/dict-types",
		`{"dictName":"用户状态","dictType":"user_status","remark":"测试字典"}`)
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	var env dictEnvelope
	require.NoError(t, json.Unmarshal(body, &env))
	var created dictTypeRespBody
	require.NoError(t, json.Unmarshal(env.Data, &created))
	require.NotZero(t, created.ID)
	require.NotNil(t, created.DictName)
	assert.Equal(t, "用户状态", *created.DictName)

	// get 回读
	status, body = doAuthRequest(t, token, "GET", ts.URL+"/api/v1/dict-types/"+int64ToStr(int64(created.ID)), "")
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	require.NoError(t, json.Unmarshal(body, &env))
	var got dictTypeRespBody
	require.NoError(t, json.Unmarshal(env.Data, &got))
	require.NotNil(t, got.DictType)
	assert.Equal(t, "user_status", *got.DictType)
	require.NotNil(t, got.Remark)
	assert.Equal(t, "测试字典", *got.Remark)

	// list 分页（envelope items/total/page/size 齐）
	status, body = doAuthRequest(t, token, "GET", ts.URL+"/api/v1/dict-types?page=1&size=10", "")
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	require.NoError(t, json.Unmarshal(body, &env))
	var page pageEnvelope
	require.NoError(t, json.Unmarshal(env.Data, &page))
	assert.GreaterOrEqual(t, page.Total, int64(1), "list 至少含刚建的类型")
	assert.Equal(t, 1, page.Page)
	assert.Equal(t, 10, page.Size)

	// update 零值保护（ENG-05）：只传 dictName，不传 remark——remark 不被清空
	status, body = doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/dict-types/"+int64ToStr(int64(created.ID)),
		`{"dictName":"用户状态改"}`)
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	require.NoError(t, json.Unmarshal(body, &env))
	var updated dictTypeRespBody
	require.NoError(t, json.Unmarshal(env.Data, &updated))
	require.NotNil(t, updated.DictName)
	assert.Equal(t, "用户状态改", *updated.DictName, "dictName 已更新")
	require.NotNil(t, updated.Remark, "零值保护：未传 remark 不清空")
	assert.Equal(t, "测试字典", *updated.Remark)

	// 重复 create（uk_dict_type 1062）→ 400/10400「字典类型已存在」
	status, body = doAuthRequest(t, token, "POST", ts.URL+"/api/v1/dict-types",
		`{"dictName":"另一个","dictType":"user_status"}`)
	require.Equal(t, http.StatusBadRequest, status, "重复 dictType 必须 400: body=%s", body)
	var dupEnv dictEnvelope
	require.NoError(t, json.Unmarshal(body, &dupEnv))
	assert.Equal(t, 10400, dupEnv.Code, "body=%s", body)
	assert.Equal(t, "字典类型已存在", dupEnv.Message, "body=%s", body)

	// update 撞 uk_dict_type（WR-02）：rename 自己为已存在的 dictType → 与 Create
	// 同冲突同翻译 400/10400，不透传 500/10001
	otherID := createDictType(t, ts, token, "其他状态", "other_status")
	status, body = doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/dict-types/"+int64ToStr(int64(created.ID)),
		`{"dictType":"other_status"}`)
	require.Equal(t, http.StatusBadRequest, status, "update 撞 uk 必须 400: body=%s", body)
	var updDupEnv dictEnvelope
	require.NoError(t, json.Unmarshal(body, &updDupEnv))
	assert.Equal(t, 10400, updDupEnv.Code, "body=%s", body)
	assert.Equal(t, "字典类型已存在", updDupEnv.Message, "body=%s", body)
	_ = otherID // 冲突源行保留至测试结束（隔离容器逐用例清库）

	// delete → get 404
	status, body = doAuthRequest(t, token, "DELETE", ts.URL+"/api/v1/dict-types/"+int64ToStr(int64(created.ID)), "")
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	status, body = doAuthRequest(t, token, "GET", ts.URL+"/api/v1/dict-types/"+int64ToStr(int64(created.ID)), "")
	require.Equal(t, http.StatusNotFound, status, "删除后 get 必须 404: body=%s", body)
	var nfEnv dictEnvelope
	require.NoError(t, json.Unmarshal(body, &nfEnv))
	assert.Equal(t, 10300, nfEnv.Code)
}

// TestDictDataWithTypeCheck 字典数据类型校验与冗余写（DICT-02）：
// 先建类型 → create data 断言 resp 与 DB 行的 dict_type_name 冗余值 →
// 对不存在 typeId create data → 404 code 10300 消息「字典类型不存在: {id}」
// （源消息原文 DictApplicationService:87）。
func TestDictDataWithTypeCheck(t *testing.T) {
	ts, dsn, adminToken, _ := newTestApp(t)
	token := adminToken()

	typeID := createDictType(t, ts, token, "性别", "gender")

	// create data：断言 resp 的 dict_type_name 冗余值
	status, body := doAuthRequest(t, token, "POST", ts.URL+"/api/v1/dict-datas",
		`{"dictTypeId":`+int64ToStr(typeID)+`,"label":"男","value":"M","sort":1}`)
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	var env dictEnvelope
	require.NoError(t, json.Unmarshal(body, &env))
	var data dictDataRespBody
	require.NoError(t, json.Unmarshal(env.Data, &data))
	require.NotNil(t, data.DictTypeName)
	assert.Equal(t, "性别", *data.DictTypeName, "dict_type_name 冗余写必须等于类型 dict_name")
	assert.Equal(t, ids.ID(typeID), data.DictTypeID)

	// DB 行断言：dict_type_name 列等于类型 dict_name 值
	db := openCheckDB(t, dsn)
	var row po.DictDataPO
	require.NoError(t, db.Where("id = ?", data.ID).First(&row).Error)
	require.NotNil(t, row.DictTypeName)
	assert.Equal(t, "性别", *row.DictTypeName, "DB 行 dict_type_name 必须冗余类型的 dict_name")

	// 不存在 typeId → 404/10300「字典类型不存在: {id}」
	const missingID = 999999
	status, body = doAuthRequest(t, token, "POST", ts.URL+"/api/v1/dict-datas",
		`{"dictTypeId":`+int64ToStr(missingID)+`,"label":"x","value":"y"}`)
	require.Equal(t, http.StatusNotFound, status, "不存在类型必须 404: body=%s", body)
	var nfEnv dictEnvelope
	require.NoError(t, json.Unmarshal(body, &nfEnv))
	assert.Equal(t, 10300, nfEnv.Code, "body=%s", body)
	assert.Equal(t, "字典类型不存在: "+int64ToStr(missingID), nfEnv.Message, "源消息原文: body=%s", body)
}

// TestDictDataFilterByType 按类型过滤（DICT-03）：两类型各 2 条 data →
// GET /api/v1/dict-datas?dict_type_id={A} 仅返回 A 的 2 条且 envelope
// items/total/page/size 齐；不传 dict_type_id 返回全量分页。
func TestDictDataFilterByType(t *testing.T) {
	ts, _, adminToken, _ := newTestApp(t)
	token := adminToken()

	typeA := createDictType(t, ts, token, "类型A", "type_a")
	typeB := createDictType(t, ts, token, "类型B", "type_b")

	for _, pair := range [][2]string{{"A1", "a1"}, {"A2", "a2"}} {
		status, body := doAuthRequest(t, token, "POST", ts.URL+"/api/v1/dict-datas",
			`{"dictTypeId":`+int64ToStr(typeA)+`,"label":"`+pair[0]+`","value":"`+pair[1]+`"}`)
		require.Equal(t, http.StatusOK, status, "body=%s", body)
	}
	for _, pair := range [][2]string{{"B1", "b1"}, {"B2", "b2"}} {
		status, body := doAuthRequest(t, token, "POST", ts.URL+"/api/v1/dict-datas",
			`{"dictTypeId":`+int64ToStr(typeB)+`,"label":"`+pair[0]+`","value":"`+pair[1]+`"}`)
		require.Equal(t, http.StatusOK, status, "body=%s", body)
	}

	// 过滤分支：仅返回 A 的 2 条
	status, body := doAuthRequest(t, token, "GET",
		ts.URL+"/api/v1/dict-datas?dict_type_id="+int64ToStr(typeA)+"&page=1&size=10", "")
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	var env dictEnvelope
	require.NoError(t, json.Unmarshal(body, &env))
	var page pageEnvelope
	require.NoError(t, json.Unmarshal(env.Data, &page))
	assert.Equal(t, int64(2), page.Total, "过滤后 total 必须 2: body=%s", body)
	require.Len(t, page.Items, 2, "过滤后 items 必须 2 条")
	for _, raw := range page.Items {
		var item dictDataRespBody
		require.NoError(t, json.Unmarshal(raw, &item))
		assert.Equal(t, ids.ID(typeA), item.DictTypeID, "过滤结果必须全属类型 A")
	}
	assert.Equal(t, 1, page.Page)
	assert.Equal(t, 10, page.Size)

	// 不过滤：全量分页（4 条）
	status, body = doAuthRequest(t, token, "GET", ts.URL+"/api/v1/dict-datas?page=1&size=10", "")
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	require.NoError(t, json.Unmarshal(body, &env))
	var allPage pageEnvelope
	require.NoError(t, json.Unmarshal(env.Data, &allPage))
	assert.Equal(t, int64(4), allPage.Total, "不过滤返回全量: body=%s", body)
}

// TestDictRoutePerm perm 三断言（D-04）：无 token 401 / 无 perm 用户 403 / admin 200。
// super_admin 种子豁免照既有 perm_integration_test 模式（matcher g 层豁免）。
func TestDictRoutePerm(t *testing.T) {
	ts, dsn, adminToken, _ := newTestApp(t)

	// ① 无 token → 401（Auth 中间件先拦）
	status, body := doRequest(t, "GET", ts.URL+"/api/v1/dict-types", "")
	require.Equal(t, http.StatusUnauthorized, status, "无 token 必须 401: body=%s", body)

	// ② 无 perm 用户（无角色）→ 403/10200
	seedUser(t, dsn, "dict-no-perm", testAdminHash, "无字典权限者", 1, false)
	uToken := loginAs(t, ts, "dict-no-perm", testAdminPass)
	status, body = doAuthRequest(t, uToken, "GET", ts.URL+"/api/v1/dict-types", "")
	require.Equal(t, http.StatusForbidden, status, "无 perm 用户必须 403: body=%s", body)
	var forbidden dictEnvelope
	require.NoError(t, json.Unmarshal(body, &forbidden))
	assert.Equal(t, 10200, forbidden.Code, "body=%s", body)

	// ③ admin（super_admin 豁免）→ 200
	status, body = doAuthRequest(t, adminToken(), "GET", ts.URL+"/api/v1/dict-types", "")
	require.Equal(t, http.StatusOK, status, "admin 必须放行: body=%s", body)
}

// configRespBody 系统配置响应体（集成断言所需子集）。
type configRespBody struct {
	ID          ids.ID  `json:"id"`
	ConfigKey   *string `json:"configKey"`
	ConfigValue *string `json:"configValue"`
}

// TestConfigCRUDAndKey 配置 CRUD 与 key 查询（CONF-01）：
// create → GET /api/v1/configs/key/{key} 断言 data 为字符串值本体 →
// 重复 create → 400/10400「配置键已存在」（uk_config_key 1062）→
// key 查询不存在 → 404 → update 零值保护 → delete。
func TestConfigCRUDAndKey(t *testing.T) {
	ts, _, adminToken, _ := newTestApp(t)
	token := adminToken()

	// create
	status, body := doAuthRequest(t, token, "POST", ts.URL+"/api/v1/configs",
		`{"configKey":"site.name","configValue":"sys-iam-go","configType":"system"}`)
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	var env dictEnvelope
	require.NoError(t, json.Unmarshal(body, &env))
	var created configRespBody
	require.NoError(t, json.Unmarshal(env.Data, &created))
	require.NotZero(t, created.ID)

	// GET /configs/key/{key}：data 为 configValue 字符串本体（JSON 形如 "data":"sys-iam-go"）
	status, body = doAuthRequest(t, token, "GET", ts.URL+"/api/v1/configs/key/site.name", "")
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	require.NoError(t, json.Unmarshal(body, &env))
	var strVal string
	require.NoError(t, json.Unmarshal(env.Data, &strVal), "data 必须为字符串本体非对象: body=%s", body)
	assert.Equal(t, "sys-iam-go", strVal, "key 查询必须返回 configValue 字符串本体")

	// 重复 create（uk_config_key 1062）→ 400/10400「配置键已存在」
	status, body = doAuthRequest(t, token, "POST", ts.URL+"/api/v1/configs",
		`{"configKey":"site.name","configValue":"dup"}`)
	require.Equal(t, http.StatusBadRequest, status, "重复 configKey 必须 400: body=%s", body)
	var dupEnv dictEnvelope
	require.NoError(t, json.Unmarshal(body, &dupEnv))
	assert.Equal(t, 10400, dupEnv.Code, "body=%s", body)
	assert.Equal(t, "配置键已存在", dupEnv.Message, "body=%s", body)

	// update 撞 uk_config_key（WR-02）：rename 自己为已存在的 configKey → 与
	// Create 同冲突同翻译 400/10400，不透传 500/10001
	dupStatus, dupBody := doAuthRequest(t, token, "POST", ts.URL+"/api/v1/configs",
		`{"configKey":"other.key","configValue":"v","configType":"system"}`)
	require.Equal(t, http.StatusOK, dupStatus, "冲突源行创建失败: body=%s", dupBody)
	var dupCreated configRespBody
	var dupCreatedEnv dictEnvelope
	require.NoError(t, json.Unmarshal(dupBody, &dupCreatedEnv))
	require.NoError(t, json.Unmarshal(dupCreatedEnv.Data, &dupCreated))
	status, body = doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/configs/"+int64ToStr(int64(created.ID)),
		`{"configKey":"other.key"}`)
	require.Equal(t, http.StatusBadRequest, status, "update 撞 uk 必须 400: body=%s", body)
	var updDupEnv dictEnvelope
	require.NoError(t, json.Unmarshal(body, &updDupEnv))
	assert.Equal(t, 10400, updDupEnv.Code, "body=%s", body)
	assert.Equal(t, "配置键已存在", updDupEnv.Message, "body=%s", body)

	// key 查询不存在 → 404/10300
	status, body = doAuthRequest(t, token, "GET", ts.URL+"/api/v1/configs/key/no.such.key", "")
	require.Equal(t, http.StatusNotFound, status, "不存在 key 必须 404: body=%s", body)
	var nfEnv dictEnvelope
	require.NoError(t, json.Unmarshal(body, &nfEnv))
	assert.Equal(t, 10300, nfEnv.Code)

	// update 零值保护（ENG-05）：只传 configValue，configKey 不动
	status, body = doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/configs/"+int64ToStr(int64(created.ID)),
		`{"configValue":"sys-iam-go-v2"}`)
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	require.NoError(t, json.Unmarshal(body, &env))
	var updated configRespBody
	require.NoError(t, json.Unmarshal(env.Data, &updated))
	require.NotNil(t, updated.ConfigValue)
	assert.Equal(t, "sys-iam-go-v2", *updated.ConfigValue)
	require.NotNil(t, updated.ConfigKey, "零值保护：未传 configKey 不清空")
	assert.Equal(t, "site.name", *updated.ConfigKey)

	// key 查询回读新值
	status, body = doAuthRequest(t, token, "GET", ts.URL+"/api/v1/configs/key/site.name", "")
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	require.NoError(t, json.Unmarshal(body, &env))
	require.NoError(t, json.Unmarshal(env.Data, &strVal))
	assert.Equal(t, "sys-iam-go-v2", strVal)

	// delete → key 查询 404
	status, body = doAuthRequest(t, token, "DELETE", ts.URL+"/api/v1/configs/"+int64ToStr(int64(created.ID)), "")
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	status, body = doAuthRequest(t, token, "GET", ts.URL+"/api/v1/configs/key/site.name", "")
	require.Equal(t, http.StatusNotFound, status, "删除后 key 查询必须 404: body=%s", body)
}

// shiftRespBody 排班响应体（集成断言所需子集；status 为 UsageStatus 枚举形态
// 出网字符串，WR-04）。
type shiftRespBody struct {
	ID        ids.ID  `json:"id"`
	ShiftCode *string `json:"shiftCode"`
	ShiftName *string `json:"shiftName"`
	StartTime *string `json:"startTime"`
	EndTime   *string `json:"endTime"`
	Status    *string `json:"status"`
}

// TestShiftFullListNoPaging 排班全量直出（SHFT-01 源行为直译）+ CRUD（SHFT-02）：
// create 3 条 → GET /api/v1/shifts 断言 data 为长度 3 数组且响应无 items/total
// 包装 → PUT 改 start_time 后 Get 回读更新生效。status 经 UsageStatus 枚举
//（WR-04）：create 恒服务端默认 "ENABLED"；update 未知枚举值 400/10401 不静默
// 落库；update "DISABLED" 合法且回读一致。
func TestShiftFullListNoPaging(t *testing.T) {
	ts, _, adminToken, _ := newTestApp(t)
	token := adminToken()

	for _, s := range [][3]string{{"S1", "早班", "08:00:00"}, {"S2", "中班", "16:00:00"}, {"S3", "晚班", "00:00:00"}} {
		status, body := doAuthRequest(t, token, "POST", ts.URL+"/api/v1/shifts",
			`{"shiftCode":"`+s[0]+`","shiftName":"`+s[1]+`","startTime":"`+s[2]+`","endTime":"23:59:00"}`)
		require.Equal(t, http.StatusOK, status, "create shift 失败: body=%s", body)
	}

	// list：data 直接是数组（无 items/total/page/size 包装——SHFT-01 直译）。
	// data 反序列化为数组成功 + 无分页键字段，即证无 envelope（对象包装会在此失败）
	status, body := doAuthRequest(t, token, "GET", ts.URL+"/api/v1/shifts", "")
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	var env dictEnvelope
	require.NoError(t, json.Unmarshal(body, &env))
	assert.NotContains(t, string(env.Data), `"items"`, "SHFT-01：响应 data 不得出现 items 分页键")
	assert.NotContains(t, string(env.Data), `"total"`, "SHFT-01：响应 data 不得出现 total 分页键")
	var items []shiftRespBody
	require.NoError(t, json.Unmarshal(env.Data, &items), "data 必须为数组非分页 envelope: body=%s", body)
	assert.Len(t, items, 3, "全量直出 3 条")

	// PUT 改 start_time → Get 回读更新生效（SHFT-02 CRUD）
	target := items[0].ID
	status, body = doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/shifts/"+int64ToStr(int64(target)),
		`{"startTime":"09:00:00"}`)
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	require.NoError(t, json.Unmarshal(body, &env))
	var updated shiftRespBody
	require.NoError(t, json.Unmarshal(env.Data, &updated))
	require.NotNil(t, updated.StartTime)
	assert.Equal(t, "09:00:00", *updated.StartTime, "start_time 字符串直传更新")
	require.NotNil(t, updated.ShiftName, "零值保护：未传 shiftName 不清空")

	// Get 回读
	status, body = doAuthRequest(t, token, "GET", ts.URL+"/api/v1/shifts/"+int64ToStr(int64(target)), "")
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	require.NoError(t, json.Unmarshal(body, &env))
	var got shiftRespBody
	require.NoError(t, json.Unmarshal(env.Data, &got))
	require.NotNil(t, got.StartTime)
	assert.Equal(t, "09:00:00", *got.StartTime)

	// status 枚举纪律（WR-04）：create 恒服务端默认 1 → 出网 "ENABLED"
	//（create 请求体不收 status——与其他域对齐）
	require.NotNil(t, got.Status)
	assert.Equal(t, "ENABLED", *got.Status, "服务端默认 status=1 必须出网 ENABLED")

	// update 未知枚举值 → UsageStatus.UnmarshalJSON 报错 → 400/10401，不静默落库
	status, body = doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/shifts/"+int64ToStr(int64(target)),
		`{"status":42}`)
	require.Equal(t, http.StatusBadRequest, status, "未知枚举值必须 400: body=%s", body)
	var badEnumEnv dictEnvelope
	require.NoError(t, json.Unmarshal(body, &badEnumEnv))
	assert.Equal(t, 10401, badEnumEnv.Code, "body=%s", body)
	assert.Contains(t, badEnumEnv.Message, "参数校验失败", "body=%s", body)

	// update 合法枚举 name 形态 → 200 且回读 "DISABLED"
	status, body = doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/shifts/"+int64ToStr(int64(target)),
		`{"status":"DISABLED"}`)
	require.Equal(t, http.StatusOK, status, "合法枚举必须 200: body=%s", body)
	require.NoError(t, json.Unmarshal(body, &env))
	var updatedStatus shiftRespBody
	require.NoError(t, json.Unmarshal(env.Data, &updatedStatus))
	require.NotNil(t, updatedStatus.Status)
	assert.Equal(t, "DISABLED", *updatedStatus.Status, "update DISABLED 必须落库并回读")

	// update 合法枚举数字双形态 → 200 回读 "ENABLED"（双形态兼容，WR-03 先例）
	status, body = doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/shifts/"+int64ToStr(int64(target)),
		`{"status":1}`)
	require.Equal(t, http.StatusOK, status, "数字双形态必须 200: body=%s", body)
	require.NoError(t, json.Unmarshal(body, &env))
	var updatedNum shiftRespBody
	require.NoError(t, json.Unmarshal(env.Data, &updatedNum))
	require.NotNil(t, updatedNum.Status)
	assert.Equal(t, "ENABLED", *updatedNum.Status)
}
