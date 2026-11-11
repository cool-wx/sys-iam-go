//go:build integration

// Fault 域集成测试：CRUD 全链（服务端默认值 -1/3 直查 DB 断言 +
// 零值保护 + perm 三断言）与 Excel 三键 upsert 导入（构造对等回测：
// 真导出文件不可得，按契约 8 列列序构造，照抄不重排）。
package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"
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

// faultImportEnvelope 导入响应 envelope（ImportResult 契约：insertCount/
// updateCount，FEATURES.md:205）。
type faultImportEnvelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		InsertCount int `json:"insertCount"`
		UpdateCount int `json:"updateCount"`
	} `json:"data"`
}

// faultImportHeaders 契约表头 8 列（测试构造器与 service 共用契约——文案与列序
// 均照契约逐字，不发明格式）。
var faultImportHeaders = []string{"故障代码", "故障变量", "触发位", "报警类型", "故障描述", "故障原因", "故障位置", "解决方法"}

// buildFaultXlsx 构造契约格式 xlsx 字节流（fallback 落地）：首行 8 列契约
// 表头（文案逐字）+ 数据行。数据行元素 nil = 不写该单元格（GetRows 行尾空单元
// 格跳过——顺带覆盖解析侧 len(row)>idx 边界分支）。headers 可注入变体表头
// （表头拒绝测试用），nil 时用契约表头。
func buildFaultXlsx(t *testing.T, headers []string, rows [][]any) []byte {
	t.Helper()
	if headers == nil {
		headers = faultImportHeaders
	}
	f := excelize.NewFile()
	defer f.Close()
	for i, h := range headers {
		cell, err := excelize.CoordinatesToCellName(i+1, 1)
		require.NoError(t, err)
		require.NoError(t, f.SetCellValue("Sheet1", cell, h))
	}
	for r, row := range rows {
		for c, v := range row {
			if v == nil {
				continue // 空单元格不写（行尾短行形态）
			}
			cell, err := excelize.CoordinatesToCellName(c+1, r+2)
			require.NoError(t, err)
			require.NoError(t, f.SetCellValue("Sheet1", cell, v))
		}
	}
	var buf bytes.Buffer
	require.NoError(t, f.Write(&buf))
	return buf.Bytes()
}

// faultRows 直查 sys_fault_info 行数。
func faultRows(t *testing.T, dsn string) int64 {
	t.Helper()
	db := openCheckDB(t, dsn)
	var cnt int64
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM sys_fault_info").Scan(&cnt).Error)
	return cnt
}

// TestFaultImportUpsert 三键 upsert 主链：库预置行 A（name=V1/bit=5/
// location=L1）；构造 xlsx 三行——行 1 与 A 三键全同（fix 改新值）/行 2 新三键/
// 行 3 name 空 → 导入 200 断言 insertCount=1、updateCount=1；直查 DB：A 行 id
// 不变且 fix 已更新（UPDATE 复用 id 直证）、新行存在、恰 2 行总计（空 name 行
// 未入库）。
func TestFaultImportUpsert(t *testing.T) {
	ts, dsn, adminToken, _ := newTestApp(t)
	token := adminToken()

	// 预置行 A：经 API create（bitIndex=5/location=L1 显式传入）
	preID := mustCreate(t, token, ts.URL+"/api/v1/faults",
		`{"code":"A1","name":"V1","bitIndex":5,"location":"L1","fix":"旧fix"}`)
	require.Equal(t, int64(1), faultRows(t, dsn))

	// 构造 xlsx 三行（契约 8 列；行 3 故障变量空 → skip）
	content := buildFaultXlsx(t, nil, [][]any{
		{"E1", "V1", 5, "故障", "d1", "r1", "L1", "新fix值"},
		{"E2", "VNEW", 9, "警告", "d2", "r2", "L2", "f2"},
		{"E3", nil, 7, "消息", "d3", "r3", "L3", "f3"}, // name 空 → skip 不计数
	})
	status, body := doUpload(t, token, ts.URL+"/api/v1/faults/import", "故障库.xlsx", content, "")
	require.Equal(t, http.StatusOK, status, "导入必须成功: body=%s", body)
	var env faultImportEnvelope
	require.NoError(t, json.Unmarshal(body, &env), "body=%s", body)
	require.Equal(t, 0, env.Code, "body=%s", body)
	assert.Equal(t, 1, env.Data.InsertCount, "仅新三键行 INSERT（body=%s）", body)
	assert.Equal(t, 1, env.Data.UpdateCount, "三键全同行 UPDATE（body=%s）", body)

	// 直查 DB：A 行 id 不变且 fix 已更新（UPDATE 复用 id 直证）
	checkDB := openCheckDB(t, dsn)
	var rowA struct {
		ID  int64
		Fix *string
	}
	require.NoError(t, checkDB.Raw("SELECT id, fix FROM sys_fault_info WHERE name = 'V1'").Scan(&rowA).Error)
	assert.Equal(t, preID, strconv.FormatInt(rowA.ID, 10), "UPDATE 必须复用原行 id（name=V1 行不得重插）")
	require.NotNil(t, rowA.Fix)
	assert.Equal(t, "新fix值", *rowA.Fix, "Excel 行 1 的 fix 必须更新到 A 行")

	// 恰 2 行总计：新行存在 + 空 name 行未入库
	assert.Equal(t, int64(2), faultRows(t, dsn), "空 name 行必须 skip 不入库")
	var cntNew int64
	require.NoError(t, checkDB.Raw("SELECT COUNT(*) FROM sys_fault_info WHERE name = 'VNEW'").Scan(&cntNew).Error)
	assert.Equal(t, int64(1), cntNew, "新三键行必须 INSERT 一行")
	var cntEmpty int64
	require.NoError(t, checkDB.Raw("SELECT COUNT(*) FROM sys_fault_info WHERE name IS NULL OR name = ''").Scan(&cntEmpty).Error)
	assert.Equal(t, int64(0), cntEmpty, "name 空行不得入库")
}

// TestFaultImportLocationNullNoDedup location NULL 判重语义锁定（断言形态）：
// 同一三键（name=V2/bit=1/location 空）连续导入
// 两次 → 两次均 insertCount=1 → 库中 V2 两行。location 为 NULL 时 eq 判重恒
// 不中、反复 INSERT 是既定的 eq 直传行为（三键查询 NULL 边界特性）
// ——**这是语义锁定测试，非缺陷**：勿改 service 为 IS NULL 双分支
// 使本测试变红。
func TestFaultImportLocationNullNoDedup(t *testing.T) {
	ts, dsn, adminToken, _ := newTestApp(t)
	token := adminToken()

	rows := [][]any{{"E1", "V2", 1, "消息", nil, nil, nil, nil}} // location 单元格不写 → 空 → NULL
	content := buildFaultXlsx(t, nil, rows)

	for i := 1; i <= 2; i++ {
		status, body := doUpload(t, token, ts.URL+"/api/v1/faults/import", "故障库.xlsx", content, "")
		require.Equal(t, http.StatusOK, status, "第 %d 次导入必须成功: body=%s", i, body)
		var env faultImportEnvelope
		require.NoError(t, json.Unmarshal(body, &env), "body=%s", body)
		require.Equal(t, 0, env.Code)
		assert.Equal(t, 1, env.Data.InsertCount, "第 %d 次导入：location NULL 判重恒不中 → 必须 INSERT（Q2 照源语义）", i)
		assert.Equal(t, 0, env.Data.UpdateCount, "第 %d 次导入：eq(location=NULL) 恒 false → 不得 UPDATE", i)
	}

	// 直查 DB：V2 恰两行且 location 均 NULL
	checkDB := openCheckDB(t, dsn)
	var cnt int64
	require.NoError(t, checkDB.Raw("SELECT COUNT(*) FROM sys_fault_info WHERE name = 'V2'").Scan(&cnt).Error)
	assert.Equal(t, int64(2), cnt, "同三键（location NULL）二次导入必须两行——源既有行为锁定")
	var nullLoc int64
	require.NoError(t, checkDB.Raw("SELECT COUNT(*) FROM sys_fault_info WHERE name = 'V2' AND location IS NULL").Scan(&nullLoc).Error)
	assert.Equal(t, int64(2), nullLoc, "location 空单元格导入后该列必须为 NULL")
}

// TestFaultImportTxRollback 整批单事务回滚直证：合法 2 行 +
// 第 3 行触发位非数字 → 导入 400 → 直查 DB 行数与导入前一致（全批无痕回滚——
// 前 2 行合法数据不得部分落库）。
func TestFaultImportTxRollback(t *testing.T) {
	ts, dsn, adminToken, _ := newTestApp(t)
	token := adminToken()
	require.Equal(t, int64(0), faultRows(t, dsn), "前置：库中 0 行")

	content := buildFaultXlsx(t, nil, [][]any{
		{"E1", "OK1", 1, "消息", "d", "r", "L", "f"},
		{"E2", "OK2", 2, "警告", "d", "r", "L", "f"},
		{"E3", "BAD", "xyz", "故障", "d", "r", "L", "f"}, // 触发位非数字 → 行错误上抛
	})
	status, body := doUpload(t, token, ts.URL+"/api/v1/faults/import", "故障库.xlsx", content, "")
	require.Equal(t, http.StatusBadRequest, status, "任一行解析失败必须整批拒绝: body=%s", body)
	var env faultImportEnvelope
	require.NoError(t, json.Unmarshal(body, &env))
	assert.Equal(t, 10400, env.Code, "body=%s", body)
	assert.Contains(t, env.Message, "触发位必须为整数", "错误消息应指明行列: body=%s", body)

	assert.Equal(t, int64(0), faultRows(t, dsn), "整批回滚：前 2 行合法数据不得部分落库（DB 行数与导入前一致）")
}

// TestFaultImportHeaderReject 表头严格匹配（防线）：表头第 2 列文案换错 /
// 列序对调两种变体 → 均 400 且 DB 零行（不产生任何行）。
func TestFaultImportHeaderReject(t *testing.T) {
	ts, dsn, adminToken, _ := newTestApp(t)
	token := adminToken()

	// 变体 1：第 2 列文案换错（故障变量 → 变量名）
	wrongText := []string{"故障代码", "变量名", "触发位", "报警类型", "故障描述", "故障原因", "故障位置", "解决方法"}
	// 变体 2：列 1/2 对调（列序不符——禁重排）
	swapped := []string{"故障变量", "故障代码", "触发位", "报警类型", "故障描述", "故障原因", "故障位置", "解决方法"}
	for name, hdr := range map[string][]string{"文案换错": wrongText, "列序对调": swapped} {
		content := buildFaultXlsx(t, hdr, [][]any{{"E1", "V1", 1, "消息", "d", "r", "L", "f"}})
		status, body := doUpload(t, token, ts.URL+"/api/v1/faults/import", "故障库.xlsx", content, "")
		require.Equal(t, http.StatusBadRequest, status, "表头%s必须整体拒绝: body=%s", name, body)
		var env faultImportEnvelope
		require.NoError(t, json.Unmarshal(body, &env))
		assert.Equal(t, 10400, env.Code, "%s: body=%s", name, body)
		assert.Equal(t, "Excel 表头与模板不符", env.Message, "%s 拒绝消息: body=%s", name, body)
	}
	assert.Equal(t, int64(0), faultRows(t, dsn), "表头不符的文件不得产生任何行")

	// 表头行不足 8 列（行尾空单元格被 GetRows 跳过 → 短表头）同样拒绝
	shortHdr := []string{"故障代码", "故障变量", "触发位", "报警类型"}
	content := buildFaultXlsx(t, shortHdr, [][]any{{"E1", "V1", 1, "消息"}})
	status, body := doUpload(t, token, ts.URL+"/api/v1/faults/import", "故障库.xlsx", content, "")
	require.Equal(t, http.StatusBadRequest, status, "短表头必须拒绝: body=%s", body)
	assert.Equal(t, int64(0), faultRows(t, dsn))
}

// TestFaultImportRowLimit 行数上限防线：构造 5001 行（表头+5000
// 数据）→ 400 整批拒绝（上限含表头计；超限文件不进逐行解析）。
func TestFaultImportRowLimit(t *testing.T) {
	ts, dsn, adminToken, _ := newTestApp(t)
	token := adminToken()

	// 5000 数据行循环生成（内存可控——单行 8 单元格）
	rows := make([][]any, 0, 5000)
	for i := 0; i < 5000; i++ {
		rows = append(rows, []any{"E", "VL", i, "消息", nil, nil, nil, nil})
	}
	content := buildFaultXlsx(t, nil, rows)
	status, body := doUpload(t, token, ts.URL+"/api/v1/faults/import", "故障库.xlsx", content, "")
	require.Equal(t, http.StatusBadRequest, status, "5001 行（含表头）必须拒绝: body=%s", body)
	var env faultImportEnvelope
	require.NoError(t, json.Unmarshal(body, &env))
	assert.Equal(t, 10400, env.Code, "body=%s", body)
	assert.Contains(t, env.Message, "导入数据超过上限 5000 行", "body=%s", body)
	assert.Equal(t, int64(0), faultRows(t, dsn), "超限文件不得产生任何行")

	// 边界内（5000 行含表头 = 4999 数据行）必须放行——上限是"含表头 > 5000 拒绝"
	contentOK := buildFaultXlsx(t, nil, rows[:4999])
	status, body = doUpload(t, token, ts.URL+"/api/v1/faults/import", "故障库.xlsx", contentOK, "")
	require.Equal(t, http.StatusOK, status, "恰 5000 行（含表头）必须放行: body=%s", body)
	require.NoError(t, json.Unmarshal(body, &env))
	assert.Equal(t, 4999, env.Data.InsertCount, "body=%s", body)
}

// TestFaultImportYf175aData yf175a 真实数据冒烟（增强项，非验收硬门槛）：
// 读 tims 项目 77 行真实 PLC 故障数据（人工原始资料，列序与契约不同），按列头
// 名称映射转换为契约格式构造（真数据+构造格式）→ 导入断言 insertCount≥70。
// 数据源文件不存在时 t.Skip（离线环境）。
func TestFaultImportYf175aData(t *testing.T) {
	const yfPath = "/home/wx/workspace/tims/backend/docs/yf175a故障信息表.xlsx"
	if _, err := os.Stat(yfPath); err != nil {
		t.Skip("yf175a 真实数据文件不可得，跳过真数据冒烟（构造数据回测已覆盖）")
	}
	ts, _, adminToken, _ := newTestApp(t)
	token := adminToken()

	src, err := excelize.OpenFile(yfPath)
	require.NoError(t, err)
	defer src.Close()
	srcRows, err := src.GetRows(src.GetSheetList()[0])
	require.NoError(t, err)
	require.Greater(t, len(srcRows), 1, "数据源必须含数据行")

	// 按列头名称定位列（序号/备注等冗余列丢弃；映射即 8 列契约的子集）
	srcIdx := map[string]int{}
	for i, h := range srcRows[0] {
		srcIdx[h] = i
	}
	pick := func(row []string, col string) any {
		if j, ok := srcIdx[col]; ok && len(row) > j && row[j] != "" {
			return row[j]
		}
		return nil
	}
	conv := make([][]any, 0, len(srcRows)-1)
	for _, row := range srcRows[1:] {
		conv = append(conv, []any{
			pick(row, "故障代码"), pick(row, "故障变量"), pick(row, "触发位"), pick(row, "报警类型"),
			pick(row, "故障描述"), pick(row, "故障原因"), pick(row, "故障位置"), pick(row, "解决方法"),
		})
	}
	content := buildFaultXlsx(t, nil, conv)

	status, body := doUpload(t, token, ts.URL+"/api/v1/faults/import", "故障库.xlsx", content, "")
	require.Equal(t, http.StatusOK, status, "真数据导入必须成功: body=%s", body)
	var env faultImportEnvelope
	require.NoError(t, json.Unmarshal(body, &env), "body=%s", body)
	require.Equal(t, 0, env.Code, "body=%s", body)
	assert.GreaterOrEqual(t, env.Data.InsertCount, 70, "77 行真实数据（探针实测 name 无空/触发位全数字）应绝大多数入库: body=%s", body)
}

// TestFaultCRUD 故障 CRUD 全链：create（服务端默认 bitIndex=-1/
// type=3(UNKNOWN)——resp 与直查 DB 双断言，非 NULL）→ get → list 分页断言 →
// update 零值保护（不传 fix 不清空）→ delete → get 404；perm 三断言
// （无 token 401 / 无 perm 用户 403 / admin 200——fault 为发明串
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
		Code int           `json:"code"`
		Data faultRespBody `json:"data"`
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
	// 绕过列默认，服务端默认值语义必须真实落库，验收面）---
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
	require.NotNil(t, updated.Data.Fix, "不传 fix 必须不清空（nil-skip 零值保护）")
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
