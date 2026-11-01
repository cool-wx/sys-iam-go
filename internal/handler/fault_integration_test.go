//go:build integration

// Fault 域集成测试（FAUL-01/02）：CRUD 全链（服务端默认值 -1/3 直查 DB 断言 +
// 零值保护 + perm 三断言）与 Excel 三键 upsert 导入（构造等价回测，D-14 裁决：
// 真导出文件不可得，按源 @ExcelProperty 列序构造——列序表 04-RESEARCH
// Implementation Guidance「Excel 列序表」，照抄不重排）。
package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// faultRespBody 故障信息响应体（集成断言所需子集；type 为 name 字符串——
// types.FaultType MarshalJSON 出网 name）。
type faultRespBody struct {
	ID       string  `json:"id"`
	Code     *string `json:"code"`
	Name     *string `json:"name"`
	BitIndex *int    `json:"bitIndex"`
	Type     *string `json:"type"`
	Fix      *string `json:"fix"`
}

// TestFaultCRUD 故障 CRUD 全链（FAUL-01）：create（服务端默认 bitIndex=-1/
// type=3(UNKNOWN)——resp 与直查 DB 双断言，非 NULL）→ get → list 分页断言 →
// update 零值保护（不传 fix 不清空）→ delete → get 404；perm 三断言
// （无 token 401 / 无 perm 用户 403 / admin 200，D-04/D-16——fault 为发明串
// 仅 super_admin 可用边界同 shift 先例）。
func TestFaultCRUD(t *testing.T) {
	ts, dsn, adminToken, _ := newTestApp(t)
	token := adminToken()

	// --- perm 三断言：① 无 token → 401 ---
	status, body := doRequest(t, "GET", ts.URL+"/api/v1/faults", "")
	require.Equal(t, http.StatusUnauthorized, status, "无 token 必须 401: body=%s", body)

	// ② 无 perm 用户（无角色）→ 403/10200
	seedUser(t, dsn, "fault-no-perm", testAdminHash, "无故障权限者", 1, false)
	uToken := loginAs(t, ts, "fault-no-perm", testAdminPass)
	status, body = doAuthRequest(t, uToken, "GET", ts.URL+"/api/v1/faults", "")
	require.Equal(t, http.StatusForbidden, status, "无 perm 用户必须 403: body=%s", body)
	var forbidden struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(body, &forbidden))
	assert.Equal(t, 10200, forbidden.Code, "body=%s", body)

	// --- create（不传 bitIndex/type）：resp 断言服务端默认 -1/3 ---
	status, body = doAuthRequest(t, token, "POST", ts.URL+"/api/v1/faults",
		`{"code":"E1001","name":"电机过温","detail":"电机温度超过阈值","fix":"停机降温"}`)
	require.Equal(t, http.StatusOK, status, "create 失败: body=%s", body)
	var created struct {
		Code int            `json:"code"`
		Data faultRespBody  `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &created), "body=%s", body)
	require.Equal(t, 0, created.Code, "body=%s", body)
	faultID := created.Data.ID
	require.NotEmpty(t, faultID)
	require.NotNil(t, created.Data.BitIndex)
	assert.Equal(t, -1, *created.Data.BitIndex, "bitIndex 未传必须服务端默认 -1（源字段初始值）")
	require.NotNil(t, created.Data.Type)
	assert.Equal(t, "UNKNOWN", *created.Data.Type, "type 未传必须服务端默认 3(UNKNOWN)")

	// --- 直查 DB：bit_index/type 落库值 -1/3（非 NULL——GORM nil 指针写 NULL 会
	// 绕过列默认，服务端默认值语义必须真实落库，FAUL-01 验收面）---
	checkDB := openCheckDB(t, dsn)
	var dbRow struct {
		BitIndex *int `gorm:"column:bit_index"`
		Type     *int8
	}
	require.NoError(t, checkDB.Raw("SELECT bit_index, type FROM sys_fault_info WHERE id = ?", faultID).Scan(&dbRow).Error)
	require.NotNil(t, dbRow.BitIndex, "bit_index 不得为 NULL（服务端默认而非 DB DEFAULT）")
	assert.Equal(t, -1, *dbRow.BitIndex)
	require.NotNil(t, dbRow.Type, "type 不得为 NULL（服务端默认而非 DB DEFAULT）")
	assert.Equal(t, int8(3), *dbRow.Type)

	// --- get：回显一致 ---
	status, body = doAuthRequest(t, token, "GET", ts.URL+"/api/v1/faults/"+faultID, "")
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	var got struct {
		Data faultRespBody `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &got))
	assert.Equal(t, faultID, got.Data.ID)
	require.NotNil(t, got.Data.Name)
	assert.Equal(t, "电机过温", *got.Data.Name)

	// --- list 分页断言（1/15 全局默认 + total=1）---
	status, body = doAuthRequest(t, token, "GET", ts.URL+"/api/v1/faults?page=1&size=15", "")
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	var listEnv struct {
		Data struct {
			Items []faultRespBody `json:"items"`
			Total int64           `json:"total"`
			Page  int             `json:"page"`
			Size  int             `json:"size"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &listEnv))
	require.Len(t, listEnv.Data.Items, 1)
	assert.Equal(t, int64(1), listEnv.Data.Total)
	assert.Equal(t, 1, listEnv.Data.Page)
	assert.Equal(t, 15, listEnv.Data.Size)

	// --- code eq 直传过滤：命中/未命中 ---
	status, body = doAuthRequest(t, token, "GET", ts.URL+"/api/v1/faults?code=E1001", "")
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	require.NoError(t, json.Unmarshal(body, &listEnv))
	assert.Equal(t, int64(1), listEnv.Data.Total, "code eq 过滤应命中 1 行")
	status, body = doAuthRequest(t, token, "GET", ts.URL+"/api/v1/faults?code=NOPE", "")
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	require.NoError(t, json.Unmarshal(body, &listEnv))
	assert.Equal(t, int64(0), listEnv.Data.Total, "code eq 过滤未命中应 0 行")

	// --- update 零值保护：仅传 code，不传 fix → fix 不清空 ---
	status, body = doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/faults/"+faultID,
		`{"code":"E1002"}`)
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	var updated struct {
		Data faultRespBody `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &updated))
	require.NotNil(t, updated.Data.Fix, "不传 fix 必须不清空（nil-skip 零值保护，ENG-05）")
	assert.Equal(t, "停机降温", *updated.Data.Fix)
	require.NotNil(t, updated.Data.Code)
	assert.Equal(t, "E1002", *updated.Data.Code, "显式传 code 必须更新")

	// --- update 不存在 id → 404 域消息 ---
	status, body = doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/faults/99004", `{"code":"X"}`)
	require.Equal(t, http.StatusNotFound, status, "body=%s", body)

	// --- delete → 200；get → 404 ---
	status, body = doAuthRequest(t, token, "DELETE", ts.URL+"/api/v1/faults/"+faultID, "")
	require.Equal(t, http.StatusOK, status, "delete 失败: body=%s", body)
	status, body = doAuthRequest(t, token, "GET", ts.URL+"/api/v1/faults/"+faultID, "")
	require.Equal(t, http.StatusNotFound, status, "删除后 get 必须 404: body=%s", body)

	// --- perm 三断言：③ admin（super_admin 豁免）→ 200 ---
	status, body = doAuthRequest(t, adminToken(), "GET", ts.URL+"/api/v1/faults", "")
	require.Equal(t, http.StatusOK, status, "admin 必须放行: body=%s", body)
}
