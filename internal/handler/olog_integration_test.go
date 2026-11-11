//go:build integration

// 操作日志域集成测试（真实 MySQL+Redis 双容器，复用 newTestApp
// 基建）。覆盖：
//   - TestOpLogWriteRecorded：写路由落库行七要素（module/operation/method/
//     params/user_id/status/error_msg）+ body 回放不破坏业务接口（200 双断言）
//   - TestOpLogMaskedParams：8 敏感字段字符串值落库前掩码 ******、普通字段原样
//     （集成直证，逐字段保真）
//   - TestOpLogFailedRequestRecorded：失败请求 status=0 + error_msg 非空截 500
//   - TestOpLogLoginNullIdentity：login 401 也落库且 user_id NULL（未登录分支）
//   - TestOpLogListPagination：GET /api/v1/logs 缺省 1/10 特例（非全局 1/15）、
//     module like 模糊、real_name 无命中空 []、第二页 create_time 倒序
package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cool-wx/sys-iam-go/internal/app"
	"github.com/cool-wx/sys-iam-go/internal/model/po"
	"github.com/cool-wx/sys-iam-go/pkg/ids"
	"github.com/cool-wx/sys-iam-go/pkg/types"
)

// testApplication 当前测试的 App 实例（newTestApp 内赋值——同包测试串行执行，
// 供 Olog FlushOpLogs 同步断言；与 testRedisAddr 同形态的包级注入）。
var testApplication *app.App

// ologEnvelope 响应包装（同 dict 测试形态；data 延迟解析）。
type ologEnvelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// ologPageEnvelope 分页 envelope（items/total/page/size）。
type ologPageEnvelope struct {
	Items []json.RawMessage `json:"items"`
	Total int64             `json:"total"`
	Page  int               `json:"page"`
	Size  int               `json:"size"`
}

// ologRespBody 操作日志响应体（集成断言所需子集）。
type ologRespBody struct {
	ID         *ids.ID              `json:"id"`
	UserID     *ids.ID              `json:"userId"`
	RealName   *string              `json:"realName"`
	Module     *string              `json:"module"`
	Operation  *string              `json:"operation"`
	Method     *string              `json:"method"`
	Params     *string              `json:"params"`
	Status     *int8                `json:"status"`
	ErrorMsg   *string              `json:"errorMsg"`
	CreateTime *types.LocalDateTime `json:"createTime"`
}

// flushOpLogs 同步排空操作日志 chan（Flush 语义——免 sleep 轮询）。
func flushOpLogs(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, testApplication.FlushOpLogs(ctx), "FlushOpLogs 失败")
}

// latestOlogByModule 直查 sys_operation_log 最新一条指定 module 的日志行
// （绕过 API，验证真实落库内容；checkDB 形态与 user_integration_test 一致）。
func latestOlogByModule(t *testing.T, dsn, module string) *po.OperationLogPO {
	t.Helper()
	checkDB, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	require.NoError(t, err)
	sqlDB, err := checkDB.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	var row po.OperationLogPO
	require.NoError(t, checkDB.Where("module = ?", module).
		Order("id DESC").First(&row).Error, "落库行缺失: module=%s", module)
	return &row
}

// createNDictTypes 循环创建 n 个字典类型（每次写路由触发一条 Oplog）。
// dict_type 唯一性：纳秒时间戳 + 序号兜底。
func createNDictTypes(t *testing.T, tsURL, token string, n int) {
	t.Helper()
	base := time.Now().Format("150405.000000000")
	for i := 0; i < n; i++ {
		body := `{"dictName":"类型` + base + `_` + strconv.Itoa(i) + `","dictType":"pag_` + base + `_` + strconv.Itoa(i) + `"}`
		status, respBody := doAuthRequest(t, token, "POST", tsURL+"/api/v1/dict-types", body)
		require.Equal(t, http.StatusOK, status, "预置 dict-type 失败: body=%s", respBody)
	}
}

// TestOpLogWriteRecorded 写操作落库行七要素 + body 回放不破坏业务接口（
// 双断言：接口 200 + 日志行 params 含原 body JSON）。
func TestOpLogWriteRecorded(t *testing.T) {
	ts, dsn, adminToken, adminID := newTestApp(t)
	token := adminToken()

	body := `{"dictName":"用户状态","dictType":"olog_write","remark":"测试字典"}`
	status, respBody := doAuthRequest(t, token, "POST", ts.URL+"/api/v1/dict-types", body)
	require.Equal(t, http.StatusOK, status, "业务接口必须正常 200（body 回放不破坏 handler）: body=%s", respBody)

	flushOpLogs(t)
	row := latestOlogByModule(t, dsn, "DictTypeController")

	require.NotNil(t, row.Module)
	assert.Equal(t, "DictTypeController", *row.Module)
	require.NotNil(t, row.Operation)
	assert.Equal(t, "create", *row.Operation)
	require.NotNil(t, row.Method)
	assert.Equal(t, "POST /api/v1/dict-types", *row.Method)
	require.NotNil(t, row.UserID)
	assert.Equal(t, adminID, *row.UserID, "user_id 必须取会话 identity")
	require.NotNil(t, row.RealName)
	assert.Equal(t, "管理员", *row.RealName)
	require.NotNil(t, row.Params)
	assert.Contains(t, *row.Params, `"dictName":"用户状态"`, "params 必须回放原 body JSON")
	assert.Contains(t, *row.Params, `"dictType":"olog_write"`)
	require.NotNil(t, row.Status)
	assert.Equal(t, int8(1), *row.Status, "200 请求 status=1")
	assert.Nil(t, row.ErrorMsg, "成功请求 error_msg 为 NULL")
	require.NotNil(t, row.IP)
	assert.NotEmpty(t, *row.IP)
	require.NotNil(t, row.Duration)
	assert.GreaterOrEqual(t, *row.Duration, 0)
}

// TestOpLogMaskedParams 8 敏感字段字符串值落库前掩码（集成直证）：
// params 无任何明文敏感值、8 处 ******、普通字段原样保留。
func TestOpLogMaskedParams(t *testing.T) {
	ts, dsn, adminToken, _ := newTestApp(t)
	token := adminToken()

	// dict-types 接口忽略未知键（binding 只拦缺必填），敏感键随 body 直达中间件
	body := `{"dictName":"掩码测试","dictType":"olog_masked",` +
		`"password":"p1","oldPassword":"o1","newPassword":"n1","secretKey":"s1",` +
		`"apiKey":"k1","token":"t1","accessToken":"a1","refreshToken":"r1",` +
		`"remark":"普通字段保留"}`
	status, respBody := doAuthRequest(t, token, "POST", ts.URL+"/api/v1/dict-types", body)
	require.Equal(t, http.StatusOK, status, "body=%s", respBody)

	flushOpLogs(t)
	row := latestOlogByModule(t, dsn, "DictTypeController")

	require.NotNil(t, row.Params)
	params := *row.Params
	for _, secret := range []string{"p1", "o1", "n1", "s1", "k1", "t1", "a1", "r1"} {
		assert.NotContains(t, params, `"`+secret+`"`, "敏感值明文泄漏: %s", secret)
	}
	for _, field := range []string{"password", "oldPassword", "newPassword", "secretKey",
		"apiKey", "token", "accessToken", "refreshToken"} {
		assert.Contains(t, params, `"`+field+`":"******"`, "字段 %s 未掩码", field)
	}
	assert.Equal(t, 8, strings.Count(params, "******"), "恰好 8 处掩码")
	assert.Contains(t, params, `"remark":"普通字段保留"`, "普通字段必须原样保留")
	assert.Contains(t, params, `"dictName":"掩码测试"`)
}

// TestOpLogFailedRequestRecorded 失败请求同样落库：status=0 且
// error_msg 非空且 ≤500 字符（rune 语义）。
func TestOpLogFailedRequestRecorded(t *testing.T) {
	ts, dsn, adminToken, _ := newTestApp(t)
	token := adminToken()

	// 第一次成功、第二次同 dict_type 重复 → 400/10400（uk_dict_type 1062 翻译）
	first := `{"dictName":"失败测试","dictType":"olog_fail"}`
	status, respBody := doAuthRequest(t, token, "POST", ts.URL+"/api/v1/dict-types", first)
	require.Equal(t, http.StatusOK, status, "body=%s", respBody)

	status, respBody = doAuthRequest(t, token, "POST", ts.URL+"/api/v1/dict-types", first)
	require.Equal(t, http.StatusBadRequest, status, "重复 dict_type 必须 400: body=%s", respBody)

	flushOpLogs(t)
	row := latestOlogByModule(t, dsn, "DictTypeController")

	require.NotNil(t, row.Status)
	assert.Equal(t, int8(0), *row.Status, "失败请求 status=0")
	require.NotNil(t, row.ErrorMsg)
	assert.NotEmpty(t, *row.ErrorMsg, "error_msg 必须非空")
	assert.LessOrEqual(t, len([]rune(*row.ErrorMsg)), 500, "error_msg 截 500")
	require.NotNil(t, row.Params)
	assert.Contains(t, *row.Params, `"dictType":"olog_fail"`, "失败请求 params 同样回放")
}

// TestOpLogLoginNullIdentity 未登录路由（login 自身）落库且 user_id NULL
// （identity 缺失分支；错误密码 401 也记录）。
func TestOpLogLoginNullIdentity(t *testing.T) {
	ts, dsn, _, _ := newTestApp(t)

	// 错误密码 → 401（响应经 resp.Fail → c.Error → error_msg 链路）
	status, _ := doRequest(t, "POST", ts.URL+"/api/v1/auth/login",
		`{"username":"admin","password":"wrong-pass"}`)
	require.Equal(t, http.StatusUnauthorized, status)

	flushOpLogs(t)
	row := latestOlogByModule(t, dsn, "AuthController")

	require.NotNil(t, row.Operation)
	assert.Equal(t, "login", *row.Operation)
	require.NotNil(t, row.Method)
	assert.Equal(t, "POST /api/v1/auth/login", *row.Method)
	assert.Nil(t, row.UserID, "未登录路由 user_id 必须为 NULL")
	assert.Nil(t, row.RealName)
	require.NotNil(t, row.Status)
	assert.Equal(t, int8(0), *row.Status, "401 请求 status=0")
	require.NotNil(t, row.ErrorMsg)
	assert.NotEmpty(t, *row.ErrorMsg)
}

// TestOpLogListPagination 日志分页查询：无参缺省 1/10 特例（非全局
// 1/15）、module like 模糊命中、real_name 无命中空 []、第二页 create_time 倒序。
func TestOpLogListPagination(t *testing.T) {
	ts, _, adminToken, _ := newTestApp(t)
	token := adminToken()

	// 预置 13 条写操作日志（>12：缺省 size=10 时第一页满 10、total=13）
	createNDictTypes(t, ts.URL, token, 13)
	flushOpLogs(t)

	// 无参缺省：items 长度 10、page=1、size=10（1/10 特例直证）。容器内共 14 条
	// 日志：13 条 dict 写操作 + 1 条 adminToken() 的 login（AuthController——login
	// 路由同样被记录）；翻页排序断言用 ?module=DictType 纯 dict 子集
	status, body := doAuthRequest(t, token, "GET", ts.URL+"/api/v1/logs", "")
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	var env ologEnvelope
	require.NoError(t, json.Unmarshal(body, &env))
	var page1 ologPageEnvelope
	require.NoError(t, json.Unmarshal(env.Data, &page1))
	assert.Equal(t, 10, len(page1.Items), "缺省 size 必须为 10（特例，非全局 15）")
	assert.Equal(t, int64(14), page1.Total, "13 dict + 1 login")
	assert.Equal(t, 1, page1.Page)
	assert.Equal(t, 10, page1.Size)

	// 第一页按 create_time DESC（同秒雪花 id DESC tiebreaker）：首条 id 最大
	var firstItems []ologRespBody
	for _, raw := range page1.Items {
		var r ologRespBody
		require.NoError(t, json.Unmarshal(raw, &r))
		firstItems = append(firstItems, r)
	}
	require.Len(t, firstItems, 10)
	require.NotNil(t, firstItems[0].ID)
	require.NotNil(t, firstItems[9].ID)
	assert.Greater(t, int64(*firstItems[0].ID), int64(*firstItems[9].ID),
		"第一页必须 create_time/id 倒序（最新在前）")

	// module like 模糊命中（DictType 是 DictTypeController 子串）
	status, body = doAuthRequest(t, token, "GET", ts.URL+"/api/v1/logs?module=DictType", "")
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	require.NoError(t, json.Unmarshal(body, &env))
	var filtered ologPageEnvelope
	require.NoError(t, json.Unmarshal(env.Data, &filtered))
	assert.Equal(t, int64(13), filtered.Total, "?module=DictType like 应命中全部 13 条")

	// real_name 无命中 → 空列表 items 输出 [] 非 null
	status, body = doAuthRequest(t, token, "GET", ts.URL+"/api/v1/logs?real_name=不存在的人", "")
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	require.NoError(t, json.Unmarshal(body, &env))
	assert.Contains(t, string(env.Data), `"items":[]`, "空列表必须输出 [] 非 null")
	assert.NotContains(t, string(env.Data), `"items":null`)

	// 翻页排序断言（同 URL 序列——?module=DictType 纯 dict 子集剔除 login 干扰）：
	// page=1&size=5 → dict 最新 5 条；page=2&size=5 → 第 6-10 新；第二页首条必须
	// 旧于第一页末条（create_time 倒序翻页不漂移）
	status, body = doAuthRequest(t, token, "GET", ts.URL+"/api/v1/logs?module=DictType&page=1&size=5", "")
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	require.NoError(t, json.Unmarshal(body, &env))
	var dictPage1 ologPageEnvelope
	require.NoError(t, json.Unmarshal(env.Data, &dictPage1))
	require.Len(t, dictPage1.Items, 5)
	var dictFirstLast ologRespBody
	require.NoError(t, json.Unmarshal(dictPage1.Items[4], &dictFirstLast))
	require.NotNil(t, dictFirstLast.ID)

	status, body = doAuthRequest(t, token, "GET", ts.URL+"/api/v1/logs?module=DictType&page=2&size=5", "")
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	require.NoError(t, json.Unmarshal(body, &env))
	var page2 ologPageEnvelope
	require.NoError(t, json.Unmarshal(env.Data, &page2))
	assert.Equal(t, 5, len(page2.Items))
	assert.Equal(t, 2, page2.Page)
	assert.Equal(t, 5, page2.Size)
	var secondFirst ologRespBody
	require.NoError(t, json.Unmarshal(page2.Items[0], &secondFirst))
	require.NotNil(t, secondFirst.ID)
	assert.Less(t, int64(*secondFirst.ID), int64(*dictFirstLast.ID),
		"第二页首条必须旧于第一页末条（create_time 倒序翻页不漂移）")
}

// TestOlogSurvivesStop 『SIGTERM 后日志完整落库』一体化直证：
// 写操作入队后【不手动调 FlushOpLogs】直接调真实 App.Stop()——Stop 即
// SIGTERM 处理路径的进程内模拟（与 TestGracefulShutdownDrains 一致），其内部
// srv.Shutdown 之后含 FlushOpLogs 排空（app.go Stop 次序契约），随后断言
// 日志行已完整落库。Stop 对 oplog 连续 Flush 两次的幂等面同证：
// 落库由 Stop 内 Flush 完成、行为正确，双 Flush 属幂等冗余，无需另改。
// 断言时序：API 调用全部先于 Stop（Stop 关闭 DB/Redis 连接池）；checkDB 断言
// 全部放在 Stop 之后——latestOlogByModule 以 DSN 新开独立连接，不依赖 server
// 与应用连接池存活。
func TestOlogSurvivesStop(t *testing.T) {
	ts, dsn, adminToken, adminID := newTestApp(t)
	token := adminToken()

	// 预置部门 → PUT 改名（PUT /api/v1/depts/:id 挂
	// Oplog 声明 module=DeptController operation=update——router.go:102）
	deptID := mustCreate(t, token, ts.URL+"/api/v1/depts", `{"deptName":"研发部","sort":1}`)
	status, respBody := doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/depts/"+deptID,
		`{"deptName":"研发中心"}`)
	require.Equal(t, http.StatusOK, status, "改名必须成功: body=%s", respBody)

	// 不手动调 FlushOpLogs——直接真实 Stop 路径（内含 Flush 排空）
	testApplication.Stop()

	// Stop 后直查（DSN 新开连接）：日志行已落库且要素完整
	row := latestOlogByModule(t, dsn, "DeptController")
	require.NotNil(t, row.Operation)
	assert.Equal(t, "update", *row.Operation, "最新一条应为 PUT 改名日志")
	require.NotNil(t, row.Method)
	assert.Equal(t, "PUT /api/v1/depts/"+deptID, *row.Method)
	require.NotNil(t, row.UserID)
	assert.Equal(t, adminID, *row.UserID, "user_id 必须取会话 identity")
	require.NotNil(t, row.RealName)
	assert.Equal(t, "管理员", *row.RealName, "real_name 必须完整落库（SIGTERM 排空不丢失）")
	require.NotNil(t, row.Status)
	assert.Equal(t, int8(1), *row.Status, "200 请求 status=1")
}
