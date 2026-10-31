//go:build integration

package handler_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/cool-wx/sys-iam-go/internal/app"
	"github.com/cool-wx/sys-iam-go/internal/config"
	"github.com/cool-wx/sys-iam-go/internal/testutil"
	"github.com/cool-wx/sys-iam-go/pkg/ids"
	"github.com/cool-wx/sys-iam-go/pkg/types"
)

// TestCreateUserContract create 路径端到端契约断言（真实 MySQL 8.4 容器，D-10）。
// 覆盖：envelope 三键、雪花 ID 字符串化且 >2^53、时间无时区格式（TZ=UTC 无 8h 漂移）、
// openid null 键保留、password/extToken 永不出网、DB 行审计列为 NULL、健康端点。
func TestCreateUserContract(t *testing.T) {
	ts, dsn := newTestApp(t)

	// --- create 请求 ---
	const username = "gsd_contract_user"
	reqBody := `{"username":"` + username + `","password":"pw-123456","realName":"契约验证用户","gender":"MALE","deptId":"100","deptName":"总部"}`
	statusCode, bodyBytes := doRequest(t, "POST", ts.URL+"/api/v1/users", reqBody)
	body := string(bodyBytes)

	require.Equal(t, http.StatusOK, statusCode, "HTTP 状态: body=%s", body)

	// envelope 三键：code 恒数字 0、message 恒字符串 "ok"、data 键存在
	var envelope struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(bodyBytes, &envelope), "body=%s", body)
	require.Equal(t, 0, envelope.Code, "body=%s", body)
	require.Equal(t, "ok", envelope.Message, "body=%s", body)
	require.NotNil(t, envelope.Data, "data 键必须存在（禁 omitempty）")

	// data 键保留检查（null 键照常输出，键缺失=false）
	var dataMap map[string]any
	require.NoError(t, json.Unmarshal(envelope.Data, &dataMap), "data=%s", string(envelope.Data))
	require.Contains(t, dataMap, "openid", "未赋值字段 openid 必须以 null 键输出而非键缺失")
	require.Nil(t, dataMap["openid"], "openid 应为 null")
	require.NotContains(t, dataMap, "password", "password 键永不出网（源 WRITE_ONLY）")
	require.NotContains(t, dataMap, "extToken", "extToken 键永不出网（源 WRITE_ONLY）")
	// body 子串级防泄漏（Go 断言，非 shell grep）
	assert.NotContains(t, body, "password")
	assert.NotContains(t, body, "extToken")

	// 雪花 ID：data.id 为十进制字符串且数值 > 2^53（JS Number.MAX_SAFE_INTEGER 之上）
	var data struct {
		ID         string `json:"id"`
		Username   string `json:"username"`
		RealName   string `json:"realName"`
		DeptID     string `json:"deptId"`
		DeptName   string `json:"deptName"`
		Gender     string `json:"gender"`
		Status     string `json:"status"`
		FirstLogin bool   `json:"firstLogin"`
		Builtin    bool   `json:"builtin"`
		CreateTime string `json:"createTime"`
		UpdateTime string `json:"updateTime"`
	}
	require.NoError(t, json.Unmarshal(envelope.Data, &data), "data=%s", string(envelope.Data))

	id, err := strconv.ParseInt(data.ID, 10, 64)
	require.NoError(t, err, "data.id 必须是十进制字符串，得到 %q", data.ID)
	require.Greater(t, id, int64(9007199254740992), "雪花 ID 必须大于 2^53")

	assert.Equal(t, username, data.Username)
	assert.Equal(t, "契约验证用户", data.RealName)
	assert.Equal(t, "100", data.DeptID, "deptId 双形态入参应以字符串回显")
	assert.Equal(t, "总部", data.DeptName)
	assert.Equal(t, "MALE", data.Gender, "gender 应以 name 形态回显")
	assert.Equal(t, "ENABLED", data.Status, "服务端默认 Status=1 应以 name 形态回显")
	assert.True(t, data.FirstLogin, "服务端默认 FirstLogin=true")
	assert.False(t, data.Builtin, "服务端默认 Builtin=false")

	// 时间契约：yyyy-MM-dd HH:mm:ss 无 T 无时区；TZ=UTC 下无 8 小时漂移（types.CST() 基准）
	timePattern := regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$`)
	require.Regexp(t, timePattern, data.CreateTime, "createTime 格式契约")
	require.Regexp(t, timePattern, data.UpdateTime, "updateTime 格式契约")
	createAt, err := time.ParseInLocation(types.TimeFormat, data.CreateTime, types.CST())
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now().In(types.CST()), createAt, 2*time.Minute,
		"createTime 应为 Asia/Shanghai 墙钟当下值（TZ=UTC 下无 8 小时漂移）")

	// --- DB 行核对 ---
	checkDB, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	require.NoError(t, err)
	var row struct {
		ID         int64
		CreateTime *time.Time
		UpdateTime *time.Time
		CreateByID *int64
		Status     *int8
		FirstLogin *bool
		Builtin    *bool
	}
	require.NoError(t, checkDB.Raw(
		"SELECT id, create_time, update_time, create_by_id, status, first_login, builtin FROM sys_user WHERE username = ?",
		username,
	).Scan(&row).Error)

	assert.Equal(t, id, row.ID, "落库行 id 必须等于响应 id（应用层雪花值）")
	assert.NotNil(t, row.CreateTime, "create_time 非空")
	assert.NotNil(t, row.UpdateTime, "update_time 非空")
	assert.Nil(t, row.CreateByID, "无 identity 上下文时 create_by_id 必须为 NULL")
	require.NotNil(t, row.Status)
	assert.Equal(t, int8(1), *row.Status, "服务端默认 status=1")
	require.NotNil(t, row.FirstLogin)
	assert.True(t, *row.FirstLogin, "服务端默认 first_login=true")
	require.NotNil(t, row.Builtin)
	assert.False(t, *row.Builtin, "服务端默认 builtin=false")

	// --- 健康端点 ---
	respHealth, err := http.Get(ts.URL + "/healthz")
	require.NoError(t, err)
	bodyHealth, _ := io.ReadAll(respHealth.Body)
	respHealth.Body.Close()
	assert.Equal(t, http.StatusOK, respHealth.StatusCode)
	assert.JSONEq(t, `{"status":"ok"}`, string(bodyHealth), "healthz body 仅含 status（T-1-06）")

	respReady, err := http.Get(ts.URL + "/readyz")
	require.NoError(t, err)
	bodyReady, _ := io.ReadAll(respReady.Body)
	respReady.Body.Close()
	assert.Equal(t, http.StatusOK, respReady.StatusCode)
	assert.JSONEq(t, `{"status":"ok"}`, string(bodyReady), "readyz body 仅含 status（T-1-06）")
}

// ---- update/get 路径集成测试（01-02 Task 1）----

var idsOnce sync.Once

// newTestApp 启动独立 MySQL 容器 + 完整 app 装配 + httptest server（D-10：
// 每测试独立容器互不污染）。cleanup 链：关 HTTP → 关应用连接池 → 终止容器。
func newTestApp(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	ctx := context.Background()
	dsn, stopContainer, err := testutil.RunMySQL(ctx)
	require.NoError(t, err, "mysql testcontainer 启动与 DDL 引导失败")

	cfg := &config.Config{
		Server:    config.ServerConfig{Port: 0},
		MySQL:     config.MySQLConfig{DSN: dsn, MaxOpenConns: 5, MaxIdleConns: 2, ConnMaxLifetime: time.Minute},
		Snowflake: config.SnowflakeConfig{Node: 1},
		Log:       config.LogConfig{Level: "error"},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	application, err := app.New(cfg, logger)
	require.NoError(t, err, "app 装配失败")

	// 进程级雪花单例只装配一次：跨测试共享同一节点实例，杜绝多实例同毫秒 seq 回绕撞 ID
	idsOnce.Do(func() {
		node, err := ids.NewNode(1)
		require.NoError(t, err)
		ids.SetDefault(node)
	})

	ts := httptest.NewServer(application.Engine())
	t.Cleanup(func() {
		ts.Close()
		application.Stop()
		stopContainer()
	})
	return ts, dsn
}

// doRequest 发送 HTTP 请求（body 非空时带 application/json 头），返回 (status, body)。
func doRequest(t *testing.T, method, url, body string) (int, []byte) {
	t.Helper()
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, rdr)
	require.NoError(t, err)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	bs, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, bs
}

// createRawUser 经 POST /api/v1/users 创建一行，返回雪花 id 字符串。
func createRawUser(t *testing.T, ts *httptest.Server, body string) string {
	t.Helper()
	status, respBody := doRequest(t, "POST", ts.URL+"/api/v1/users", body)
	require.Equal(t, http.StatusOK, status, "create 失败: body=%s", respBody)
	var envelope struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(respBody, &envelope), "body=%s", respBody)
	require.NotEmpty(t, envelope.Data.ID, "响应 data.id 必须存在")
	return envelope.Data.ID
}

// TestUpdateUserZeroValueProtection PUT 仅提交 realName：未提交字段（phone/email/
// deptName/gender）保持原值、realName 已变、update_time 晚于 create_time、
// 无 identity 上下文时 update_by 两列保持 NULL（ENG-05 零值保护 + ENG-02 update 侧）。
func TestUpdateUserZeroValueProtection(t *testing.T) {
	ts, dsn := newTestApp(t)

	id := createRawUser(t, ts, `{"username":"zero-prot","password":"pw-123456","realName":"原始名","phone":"13800000000","email":"orig@example.com","gender":"MALE","deptName":"原部门"}`)

	// DATETIME 秒精度：跨过 1 秒边界使 update_time 严格晚于 create_time 可判
	time.Sleep(1100 * time.Millisecond)

	status, body := doRequest(t, "PUT", ts.URL+"/api/v1/users/"+id, `{"realName":"更新后的名字"}`)
	require.Equal(t, http.StatusOK, status, "body=%s", body)

	status, body = doRequest(t, "GET", ts.URL+"/api/v1/users/"+id, "")
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	var envelope struct {
		Data struct {
			RealName   string  `json:"realName"`
			Phone      *string `json:"phone"`
			Email      *string `json:"email"`
			DeptName   *string `json:"deptName"`
			Gender     string  `json:"gender"`
			UpdateByID *string `json:"updateById"`
			CreateTime string  `json:"createTime"`
			UpdateTime string  `json:"updateTime"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &envelope), "body=%s", body)
	d := envelope.Data
	assert.Equal(t, "更新后的名字", d.RealName, "realName 应已更新")
	require.NotNil(t, d.Phone)
	assert.Equal(t, "13800000000", *d.Phone, "未提交的 phone 必须保持原值（Pitfall 3）")
	require.NotNil(t, d.Email)
	assert.Equal(t, "orig@example.com", *d.Email, "未提交的 email 必须保持原值")
	require.NotNil(t, d.DeptName)
	assert.Equal(t, "原部门", *d.DeptName, "未提交的 deptName 必须保持原值")
	assert.Equal(t, "MALE", d.Gender, "未提交的 gender 必须保持原值")
	assert.Nil(t, d.UpdateByID, "无 identity 上下文时响应 updateById 应为 null")

	ct, err := time.ParseInLocation(types.TimeFormat, d.CreateTime, types.CST())
	require.NoError(t, err)
	ut, err := time.ParseInLocation(types.TimeFormat, d.UpdateTime, types.CST())
	require.NoError(t, err)
	assert.True(t, ut.After(ct), "update_time(%s) 必须晚于 create_time(%s)", ut, ct)

	// DB 行级断言（无 identity 分支）：列值原样保留、update_by 两列 NULL
	checkDB, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	require.NoError(t, err)
	sqlDB, _ := checkDB.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	var row struct {
		RealName     *string
		Phone        *string
		Email        *string
		DeptName     *string
		Gender       *int8
		UpdateByID   *int64
		UpdateByName *string
	}
	idNum, err := strconv.ParseInt(id, 10, 64)
	require.NoError(t, err)
	require.NoError(t, checkDB.Raw(
		"SELECT real_name, phone, email, dept_name, gender, update_by_id, update_by_name FROM sys_user WHERE id = ?",
		idNum,
	).Scan(&row).Error)
	require.NotNil(t, row.RealName)
	assert.Equal(t, "更新后的名字", *row.RealName)
	require.NotNil(t, row.Phone)
	assert.Equal(t, "13800000000", *row.Phone, "DB 行 phone 必须保持原值")
	require.NotNil(t, row.Email)
	assert.Equal(t, "orig@example.com", *row.Email, "DB 行 email 必须保持原值")
	require.NotNil(t, row.DeptName)
	assert.Equal(t, "原部门", *row.DeptName, "DB 行 dept_name 必须保持原值")
	require.NotNil(t, row.Gender)
	assert.Equal(t, int8(1), *row.Gender, "DB 行 gender=MALE(1) 必须保持原值")
	assert.Nil(t, row.UpdateByID, "无 identity 时 update_by_id 必须为 NULL")
	assert.Nil(t, row.UpdateByName, "无 identity 时 update_by_name 必须为 NULL")
}

// TestUpdateUserExplicitClear PUT 显式提交 phone:""：phone 被清空为空串（非 null），
// 未提交的 email 保持原值（nil=未提供 与 空串=显式清空 双语义，Pitfall 3）。
func TestUpdateUserExplicitClear(t *testing.T) {
	ts, _ := newTestApp(t)

	id := createRawUser(t, ts, `{"username":"explicit-clear","password":"pw-123456","realName":"清空测试","phone":"13911111111","email":"clear@example.com"}`)

	status, body := doRequest(t, "PUT", ts.URL+"/api/v1/users/"+id, `{"phone":""}`)
	require.Equal(t, http.StatusOK, status, "body=%s", body)

	status, body = doRequest(t, "GET", ts.URL+"/api/v1/users/"+id, "")
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	var envelope struct {
		Data struct {
			Phone *string `json:"phone"`
			Email *string `json:"email"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &envelope), "body=%s", body)
	require.NotNil(t, envelope.Data.Phone, "显式清空后 phone 键应为空串而非 null")
	assert.Equal(t, "", *envelope.Data.Phone, "显式空串必须清空 phone（Pitfall 3）")
	require.NotNil(t, envelope.Data.Email)
	assert.Equal(t, "clear@example.com", *envelope.Data.Email, "未提交的 email 必须保持原值")
}

// TestUpdateUserNotFound PUT 不存在的 id：HTTP 404 + envelope code=10300。
func TestUpdateUserNotFound(t *testing.T) {
	ts, _ := newTestApp(t)

	status, body := doRequest(t, "PUT", ts.URL+"/api/v1/users/99001", `{"realName":"x"}`)
	require.Equal(t, http.StatusNotFound, status, "body=%s", body)
	var envelope struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(body, &envelope))
	assert.Equal(t, 10300, envelope.Code, "body=%s", body)
}

// TestGetUserInvalidId GET 非数字 :id：HTTP 400 + envelope code=10400。
func TestGetUserInvalidId(t *testing.T) {
	ts, _ := newTestApp(t)

	status, body := doRequest(t, "GET", ts.URL+"/api/v1/users/abc", "")
	require.Equal(t, http.StatusBadRequest, status, "body=%s", body)
	var envelope struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(body, &envelope))
	assert.Equal(t, 10400, envelope.Code, "body=%s", body)
}
