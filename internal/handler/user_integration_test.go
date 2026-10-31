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

	// 雪花节点装配（等价真实 main 路径）
	node, err := ids.NewNode(1)
	require.NoError(t, err)
	ids.SetDefault(node)

	ts := httptest.NewServer(application.Engine())

	t.Cleanup(func() {
		// 顺序：关测试 HTTP → 关应用连接池 → 终止容器
		ts.Close()
		application.Stop()
		stopContainer()
	})

	// --- create 请求 ---
	const username = "gsd_contract_user"
	reqBody := `{"username":"` + username + `","password":"pw-123456","realName":"契约验证用户","gender":"MALE","deptId":"100","deptName":"总部"}`
	resp, err := http.Post(ts.URL+"/api/v1/users", "application/json", strings.NewReader(reqBody))
	require.NoError(t, err, "POST /api/v1/users 失败")
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	body := string(bodyBytes)

	require.Equal(t, http.StatusOK, resp.StatusCode, "HTTP 状态: body=%s", body)

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
