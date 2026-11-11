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

// TestCreateUserContract create 路径端到端契约断言（真实 MySQL 8.4 容器）。
// 覆盖：envelope 三键、雪花 ID 字符串化且 >2^53、时间无时区格式（TZ=UTC 无 8h 漂移）、
// openid null 键保留、password/extToken 永不出网、DB 行审计列为 NULL、健康端点。
func TestCreateUserContract(t *testing.T) {
	ts, dsn, adminToken, adminID := newTestApp(t)

	// create 强制部门校验链（deptId 必填且必须存在）——seed 部门 100
	seedDept(t, dsn, 100, "总部")

	// --- create 请求 ---
	const username = "gsd_contract_user"
	reqBody := `{"username":"` + username + `","password":"pw-123456","realName":"契约验证用户","gender":"MALE","deptId":"100","deptName":"总部"}`
	statusCode, bodyBytes := doAuthRequest(t, adminToken(), "POST", ts.URL+"/api/v1/users", reqBody)
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
	// 路由收紧：create 请求必带 admin token → identity 存在 → 审计四列
	// 必须由 BeforeCreate 填充为操作者（admin）自身
	require.NotNil(t, row.CreateByID, "带 identity 的 create 必须填充 create_by_id")
	assert.Equal(t, adminID, *row.CreateByID, "create_by_id 必须等于登录操作者（admin）")
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
	assert.JSONEq(t, `{"status":"ok"}`, string(bodyHealth), "healthz body 仅含 status")

	respReady, err := http.Get(ts.URL + "/readyz")
	require.NoError(t, err)
	bodyReady, _ := io.ReadAll(respReady.Body)
	respReady.Body.Close()
	assert.Equal(t, http.StatusOK, respReady.StatusCode)
	assert.JSONEq(t, `{"status":"ok"}`, string(bodyReady), "readyz body 仅含 status")
}

// ---- update/get 路径集成测试----

var idsOnce sync.Once

// testRedisAddr 当前测试的 Redis 容器地址（newTestApp 内赋值——同包测试可读，
// 零调用点改动；openCheckRedis 等需要直查 Redis 键状态的断言经此拨号，
// 双键直查断言引入）。
var testRedisAddr string

// testStorageRoot 当前测试的本地存储根目录（newTestApp 内赋值，同包测试可读）：
// File 域集成测试经此直查 LocalStorage 落盘对象数（判重零副作用断言）。
var testStorageRoot string

// newTestApp 启动独立 MySQL+Redis 双容器（认证链依赖）+ 完整 app 装配 +
// httptest server + 种子 admin（真实存量 BCrypt 哈希：每测试独立容器互不污染）。
// 种子先于 app.New：boot 对账（app.New 内）从 sys_user_role/sys_role_menu
// 导入 casbin g/p 策略——先有存量数据后启动服务即生产冷启动形态（super_admin 角色
// + admin 关联必须在对账前落库，否则 admin 经 requirePerm 路由全量 403）。
// 返回 ts、dsn、adminToken()（每次调用新登录一枚 token——多 token 并存语义）
// 与 adminID（审计断言需要）。cleanup 链：关 HTTP → 关应用连接池 → 终止容器。
func newTestApp(t *testing.T) (*httptest.Server, string, func() string, int64) {
	t.Helper()
	ctx := context.Background()
	dsn, stopMySQL, err := testutil.RunMySQL(ctx)
	require.NoError(t, err, "mysql testcontainer 启动与 DDL 引导失败")
	redisAddr, stopRedis, err := testutil.RunRedis(ctx)
	require.NoError(t, err, "redis testcontainer 启动失败")
	testRedisAddr = redisAddr

	// 进程级雪花单例只装配一次：跨测试共享同一节点实例，杜绝多实例同毫秒 seq 回绕撞 ID
	// （移到种子之前——seedUser/seedRole 直插行依赖 ids.NewID 生成主键）
	idsOnce.Do(func() {
		node, err := ids.NewNode(1)
		require.NoError(t, err)
		ids.SetDefault(node)
	})

	cfg := &config.Config{
		Server:    config.ServerConfig{Port: 0},
		MySQL:     config.MySQLConfig{DSN: dsn, MaxOpenConns: 5, MaxIdleConns: 2, ConnMaxLifetime: time.Minute},
		Redis:     config.RedisConfig{Addr: redisAddr},
		Auth:      config.AuthConfig{SessionTimeout: time.Hour},
		Snowflake: config.SnowflakeConfig{Node: 1},
		Log:       config.LogConfig{Level: "error"},
		// File 域：driver=local + 每测试独立临时根目录（最小改动点；
		// 签名不变，落盘根经 testStorageRoot 供同包断言）
		Storage: config.StorageConfig{
			Driver: "local",
			Local:  config.LocalStorageConfig{Root: t.TempDir()},
		},
	}
	testStorageRoot = cfg.Storage.Local.Root
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// 种子三件套（boot 对账数据源，必须在 app.New 之前——对账读 sys_user_role）：
	// super_admin 内置角色 + admin 用户 + 关联。super_admin data_scope=5（
	// GET /users 挂 dataScope 过滤后，档 NULL 的 admin 会回退最窄档
	// 本人——既有全量列表断言将崩溃；档 5 无过滤保持既有行为。生产存量
	// super_admin 行的修正走既有 PUT /roles/:id，属现场部署清单项）
	superRoleID := seedRoleWithScope(t, dsn, "超级管理员", "super_admin", true, 5)
	adminID := seedUser(t, dsn, testAdminName, testAdminHash, testAdminRealName, 1, true)
	attachRoleToUser(t, dsn, adminID, superRoleID, "超级管理员")

	application, err := app.New(cfg, logger)
	require.NoError(t, err, "app 装配失败")
	testApplication = application // 供 Olog 集成测试 FlushOpLogs 同步断言

	ts := httptest.NewServer(application.Engine())
	adminToken := func() string { return loginAs(t, ts, testAdminName, testAdminPass) }
	t.Cleanup(func() {
		ts.Close()
		application.Stop()
		stopRedis()
		stopMySQL()
	})
	return ts, dsn, adminToken, adminID
}

// doRequest 发送 HTTP 请求（body 非空时带 application/json 头），返回 (status, body)。
func doRequest(t *testing.T, method, url, body string) (int, []byte) {
	t.Helper()
	return doHeaderRequest(t, nil, method, url, body)
}

// doAuthRequest doRequest 的认证变体：带 Authorization: Bearer <token> 头
// （路由收紧后受保护路由的请求载体；原 doRequest 保留——登录/白名单仍用）。
func doAuthRequest(t *testing.T, token, method, url, body string) (int, []byte) {
	t.Helper()
	headers := map[string]string{"Authorization": "Bearer " + token}
	return doHeaderRequest(t, headers, method, url, body)
}

// doHeaderRequest 发送带额外头的 HTTP 请求（body 非空时带 application/json 头）。
func doHeaderRequest(t *testing.T, headers map[string]string, method, url, body string) (int, []byte) {
	t.Helper()
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, rdr)
	require.NoError(t, err)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
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

// loginAs 以 username/password 登录拿 token（断言 200 且 token 非空）。
func loginAs(t *testing.T, ts *httptest.Server, username, password string) string {
	t.Helper()
	status, body := doRequest(t, "POST", ts.URL+"/api/v1/auth/login",
		`{"username":"`+username+`","password":"`+password+`"}`)
	require.Equal(t, http.StatusOK, status, "登录失败: body=%s", body)
	var envelope struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &envelope), "body=%s", body)
	require.NotEmpty(t, envelope.Data.Token, "登录响应必须含 token")
	return envelope.Data.Token
}

// createRawUser 经 POST /api/v1/users（带 admin token）创建一行，返回雪花 id 字符串。
func createRawUser(t *testing.T, ts *httptest.Server, token, body string) string {
	t.Helper()
	status, respBody := doAuthRequest(t, token, "POST", ts.URL+"/api/v1/users", body)
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

// openCheckDB 打开测试专用 GORM 连接（Silent 日志）+ cleanup 关闭（DB 行级断言与
// seedUser 共用，避免散落的重复 gorm.Open/Cleanup 对）。
func openCheckDB(t *testing.T, dsn string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

// execMust 执行写语句并返回影响行数（seedUser/测试种子直插 SQL 用）。
func execMust(t *testing.T, db *gorm.DB, query string, args ...any) int64 {
	t.Helper()
	res := db.Exec(query, args...)
	require.NoError(t, res.Error, "exec 失败: %s", query)
	return res.RowsAffected
}

// seedDept 直插 sys_dept 行（user create/update 强制部门校验链——
// 测试用户必须挂真实存在的部门；各测试独立容器互不污染，按需各自 seed）。
func seedDept(t *testing.T, dsn string, id int64, name string) {
	t.Helper()
	db := openCheckDB(t, dsn)
	execMust(t, db,
		"INSERT INTO sys_dept (id, dept_name, sort, status, create_time, update_time) VALUES (?, ?, 0, 1, NOW(), NOW())",
		id, name,
	)
}

// seedDeptChild seedDept 的子节点变体（档 3 子树展开断言的三层树数据面）。
func seedDeptChild(t *testing.T, dsn string, id, parentID int64, name string) {
	t.Helper()
	db := openCheckDB(t, dsn)
	execMust(t, db,
		"INSERT INTO sys_dept (id, parent_id, dept_name, sort, status, create_time, update_time) VALUES (?, ?, ?, 0, 1, NOW(), NOW())",
		id, parentID, name,
	)
}

// TestUpdateUserZeroValueProtection PUT 仅提交 realName：未提交字段（phone/email/
// deptName/gender）保持原值、realName 已变、update_time 晚于 create_time、带 identity
// 上下文时 update_by 两列填充操作者（零值保护 + update 侧）。
func TestUpdateUserZeroValueProtection(t *testing.T) {
	ts, dsn, adminToken, adminID := newTestApp(t)

	// create 强制部门校验：seed 专属部门（dept_name 由服务端查库冗余——
	// 请求体 deptName 键已被忽略，故 seed 名即最终落库值）
	seedDept(t, dsn, 101, "原部门")
	id := createRawUser(t, ts, adminToken(), `{"username":"zero-prot","password":"pw-123456","realName":"原始名","phone":"13800000000","email":"orig@example.com","gender":"MALE","deptId":"101"}`)

	// DATETIME 秒精度：跨过 1 秒边界使 update_time 严格晚于 create_time 可判
	time.Sleep(1100 * time.Millisecond)

	status, body := doAuthRequest(t, adminToken(), "PUT", ts.URL+"/api/v1/users/"+id, `{"realName":"更新后的名字"}`)
	require.Equal(t, http.StatusOK, status, "body=%s", body)

	status, body = doAuthRequest(t, adminToken(), "GET", ts.URL+"/api/v1/users/"+id, "")
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
	assert.Equal(t, "13800000000", *d.Phone, "未提交的 phone 必须保持原值")
	require.NotNil(t, d.Email)
	assert.Equal(t, "orig@example.com", *d.Email, "未提交的 email 必须保持原值")
	require.NotNil(t, d.DeptName)
	assert.Equal(t, "原部门", *d.DeptName, "未提交的 deptName 必须保持原值")
	assert.Equal(t, "MALE", d.Gender, "未提交的 gender 必须保持原值")
	// 路由收紧：更新请求必带 admin token → identity 存在 → update_by 审计填充
	require.NotNil(t, d.UpdateByID, "带 identity 的更新响应 updateById 必须非 null")
	assert.Equal(t, strconv.FormatInt(adminID, 10), *d.UpdateByID, "updateById 必须等于登录操作者（admin）")

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
	// 带 identity：update_by 两列填充操作者（admin）
	require.NotNil(t, row.UpdateByID, "带 identity 时 update_by_id 必须落库")
	assert.Equal(t, adminID, *row.UpdateByID)
	require.NotNil(t, row.UpdateByName)
	assert.Equal(t, "管理员", *row.UpdateByName, "update_by_name 必须为操作者 realName")
}

// TestUpdateUserExplicitClear PUT 显式提交 phone:""：phone 被清空为空串（非 null），
// 未提交的 email 保持原值（nil=未提供 与 空串=显式清空 双语义）。
func TestUpdateUserExplicitClear(t *testing.T) {
	ts, dsn, adminToken, _ := newTestApp(t)

	seedDept(t, dsn, 100, "总部")
	id := createRawUser(t, ts, adminToken(), `{"username":"explicit-clear","password":"pw-123456","realName":"清空测试","phone":"13911111111","email":"clear@example.com","deptId":"100"}`)

	status, body := doAuthRequest(t, adminToken(), "PUT", ts.URL+"/api/v1/users/"+id, `{"phone":""}`)
	require.Equal(t, http.StatusOK, status, "body=%s", body)

	status, body = doAuthRequest(t, adminToken(), "GET", ts.URL+"/api/v1/users/"+id, "")
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	var envelope struct {
		Data struct {
			Phone *string `json:"phone"`
			Email *string `json:"email"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &envelope), "body=%s", body)
	require.NotNil(t, envelope.Data.Phone, "显式清空后 phone 键应为空串而非 null")
	assert.Equal(t, "", *envelope.Data.Phone, "显式空串必须清空 phone")
	require.NotNil(t, envelope.Data.Email)
	assert.Equal(t, "clear@example.com", *envelope.Data.Email, "未提交的 email 必须保持原值")
}

// TestUpdateUserNotFound PUT 不存在的 id：HTTP 404 + envelope code=10300。
func TestUpdateUserNotFound(t *testing.T) {
	ts, _, adminToken, _ := newTestApp(t)

	status, body := doAuthRequest(t, adminToken(), "PUT", ts.URL+"/api/v1/users/99001", `{"realName":"x"}`)
	require.Equal(t, http.StatusNotFound, status, "body=%s", body)
	var envelope struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(body, &envelope))
	assert.Equal(t, 10300, envelope.Code, "body=%s", body)
}

// TestUpdateUserSameSecondIdempotentRetry 同秒零变更更新回归：同一秒内对
// 已存在用户连续两次提交完全相同内容（幂等重试/双击保存）——第二次 SET 值全相同
// 且 update_time 秒精度截断同值 → affected=0，必须返回 200 而非 404（0 变更 ≠
// 不存在）。跨过秒边界时 update_time 变化 → affected=1，同样 200：本测试不会因
// 时序假失败，只在"行存在却报 404"的回归上失败。
func TestUpdateUserSameSecondIdempotentRetry(t *testing.T) {
	ts, dsn, adminToken, _ := newTestApp(t)

	seedDept(t, dsn, 100, "总部")
	id := createRawUser(t, ts, adminToken(), `{"username":"same-second-retry","password":"pw","realName":"初名","deptId":"100"}`)

	status, body := doAuthRequest(t, adminToken(), "PUT", ts.URL+"/api/v1/users/"+id, `{"realName":"同秒重试值"}`)
	require.Equal(t, http.StatusOK, status, "首次更新应 200: body=%s", body)

	status, body = doAuthRequest(t, adminToken(), "PUT", ts.URL+"/api/v1/users/"+id, `{"realName":"同秒重试值"}`)
	require.Equal(t, http.StatusOK, status, "同秒零变更重复更新必须 200（行存在）: body=%s", body)
	var envelope struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(body, &envelope), "body=%s", body)
	assert.Equal(t, 0, envelope.Code, "body=%s", body)
	assert.NotEqual(t, 10300, envelope.Code, "行存在时不得返回 10300 数据不存在")

	// 行确实还在且值为提交值：GET 可查
	status, body = doAuthRequest(t, adminToken(), "GET", ts.URL+"/api/v1/users/"+id, "")
	require.Equal(t, http.StatusOK, status, "重复更新后行必须仍可查: body=%s", body)
	var getEnvelope struct {
		Data struct {
			RealName string `json:"realName"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &getEnvelope), "body=%s", body)
	assert.Equal(t, "同秒重试值", getEnvelope.Data.RealName)
}

// TestGetUserInvalidId GET 非数字 :id：HTTP 400 + envelope code=10400。
func TestGetUserInvalidId(t *testing.T) {
	ts, _, adminToken, _ := newTestApp(t)

	status, body := doAuthRequest(t, adminToken(), "GET", ts.URL+"/api/v1/users/abc", "")
	require.Equal(t, http.StatusBadRequest, status, "body=%s", body)
	var envelope struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(body, &envelope))
	assert.Equal(t, 10400, envelope.Code, "body=%s", body)
}

// ---- list 路径与完整错误面集成测试----

// TestListUsersPaging 分页 envelope 契约：GET ?page=1&size=2 → data 内
// {items,total,page,size}，items 长度 2、total 3、items[0] 为最新行
// （ORDER BY create_time DESC, id DESC 稳定排序）。
func TestListUsersPaging(t *testing.T) {
	ts, dsn, adminToken, _ := newTestApp(t)

	seedDept(t, dsn, 100, "总部")
	// 插 3 行：前两行同秒插入，第三行跨秒——排序由 create_time DESC 主导，
	// 前两行的先后由 id DESC tiebreaker 决定（雪花 id 单调递增，后插者大）
	createRawUser(t, ts, adminToken(), `{"username":"list-a","password":"pw","realName":"用户A","deptId":"100"}`)
	createRawUser(t, ts, adminToken(), `{"username":"list-b","password":"pw","realName":"用户B","deptId":"100"}`)
	time.Sleep(1100 * time.Millisecond)
	createRawUser(t, ts, adminToken(), `{"username":"list-c","password":"pw","realName":"用户C","deptId":"100"}`)

	status, body := doAuthRequest(t, adminToken(), "GET", ts.URL+"/api/v1/users?page=1&size=2", "")
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	var envelope struct {
		Data struct {
			Items []struct {
				Username string `json:"username"`
				RealName string `json:"realName"`
			} `json:"items"`
			Total int64 `json:"total"`
			Page  int   `json:"page"`
			Size  int   `json:"size"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &envelope), "body=%s", body)
	d := envelope.Data
	require.Len(t, d.Items, 2, "page=1&size=2 应返回 2 行: body=%s", body)
	assert.Equal(t, int64(4), d.Total, "total 应为 4（admin 种子行 + 测试插入 3 行）")
	assert.Equal(t, 1, d.Page)
	assert.Equal(t, 2, d.Size)
	assert.Equal(t, "list-c", d.Items[0].Username, "items[0] 应为最新插入行")
	assert.Equal(t, "list-b", d.Items[1].Username, "同秒插入的两行按 id DESC tiebreaker 排序")
}

// TestListUsersEdgeParams 边界参数：page=0&size=-1 钳位 1/15（HTTP 200 不 500）；
// 超界页码 page=999 → items 为空 JSON 数组而非 null（空输入边界）。
func TestListUsersEdgeParams(t *testing.T) {
	ts, dsn, adminToken, _ := newTestApp(t)

	seedDept(t, dsn, 100, "总部")
	createRawUser(t, ts, adminToken(), `{"username":"edge-user","password":"pw","realName":"边界","deptId":"100"}`)

	status, body := doAuthRequest(t, adminToken(), "GET", ts.URL+"/api/v1/users?page=0&size=-1", "")
	require.Equal(t, http.StatusOK, status, "page=0&size=-1 不应报错: body=%s", body)
	var envelope struct {
		Data struct {
			Items []json.RawMessage `json:"items"`
			Total int64             `json:"total"`
			Page  int               `json:"page"`
			Size  int               `json:"size"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &envelope), "body=%s", body)
	assert.Equal(t, 1, envelope.Data.Page, "page=0 应钳位为 1")
	assert.Equal(t, 15, envelope.Data.Size, "size=-1 应钳位为 15")
	assert.Equal(t, int64(2), envelope.Data.Total, "total 应为 2（admin 种子行 + edge-user）")
	assert.NotEmpty(t, envelope.Data.Items, "应有数据行")

	// 超界页码
	status, body = doAuthRequest(t, adminToken(), "GET", ts.URL+"/api/v1/users?page=999", "")
	require.Equal(t, http.StatusOK, status, "超界页码不应报错: body=%s", body)
	var envelope2 struct {
		Data struct {
			Items []json.RawMessage `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &envelope2), "body=%s", body)
	require.NotNil(t, envelope2.Data.Items, "超界页码 items 必须是空数组 [] 而非 null")
	assert.Empty(t, envelope2.Data.Items)
	assert.Contains(t, string(body), `"items":[]`, "空列表应序列化为 [] 而非 null（Anti-Pattern：items:null）")

	// 回归防护：int64 极值页码——(page-1)*size 溢出为负会被 GORM 静默省略
	// OFFSET 子句、静默返回第 1 页数据；钳位 maxPage 后必须空页且回显钳位值
	status, body = doAuthRequest(t, adminToken(), "GET", ts.URL+"/api/v1/users?page=9223372036854775807", "")
	require.Equal(t, http.StatusOK, status, "int64 极值页码不应报错: body=%s", body)
	var envelope3 struct {
		Data struct {
			Items []json.RawMessage `json:"items"`
			Page  int               `json:"page"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &envelope3), "body=%s", body)
	assert.Empty(t, envelope3.Data.Items, "极值页码必须返回空页，不得静默回退第 1 页数据: body=%s", body)
	assert.Equal(t, 1000000, envelope3.Data.Page, "极值页码回显应为钳位值 1000000: body=%s", body)
}

// TestCreateUserDuplicateUsername 重复 POST 同 username：uk_username 唯一索引
// 1062 → gorm.ErrDuplicatedKey → HTTP 400 + code 10400。
func TestCreateUserDuplicateUsername(t *testing.T) {
	ts, dsn, adminToken, _ := newTestApp(t)

	seedDept(t, dsn, 100, "总部")
	createRawUser(t, ts, adminToken(), `{"username":"dup-user","password":"pw","realName":"第一次","deptId":"100"}`)
	status, body := doAuthRequest(t, adminToken(), "POST", ts.URL+"/api/v1/users", `{"username":"dup-user","password":"pw","realName":"第二次","deptId":"100"}`)
	require.Equal(t, http.StatusBadRequest, status, "重复 username 应 400: body=%s", body)
	var envelope struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(body, &envelope))
	assert.Equal(t, 10400, envelope.Code, "重复记录应落 10400 段: body=%s", body)
}

// TestCreateUserValidation 完整错误面：缺必填 → 400 + 10401；gender=99 未知枚举
// → 不 500（兜底 UNKNOWN 落库）；请求体携带未知额外字段 → 不 400（宽容输入，
// 源 FAIL_ON_UNKNOWN_PROPERTIES=false 语义）。
func TestCreateUserValidation(t *testing.T) {
	ts, dsn, adminToken, _ := newTestApp(t)

	seedDept(t, dsn, 100, "总部")

	// 缺 username（required）→ 400 + 10401
	status, body := doAuthRequest(t, adminToken(), "POST", ts.URL+"/api/v1/users", `{"password":"pw","realName":"缺用户名"}`)
	require.Equal(t, http.StatusBadRequest, status, "缺必填应 400: body=%s", body)
	var envelope struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(body, &envelope))
	assert.Equal(t, 10401, envelope.Code, "binding 失败应落 10401 段: body=%s", body)

	// 部门校验链：deptId 缺失 → 400「部门不能为空」；deptId 不存在 → 400「部门不存在: 888」
	status, body = doAuthRequest(t, adminToken(), "POST", ts.URL+"/api/v1/users", `{"username":"no-dept","password":"pw","realName":"无部门"}`)
	require.Equal(t, http.StatusBadRequest, status, "缺 deptId 应 400: body=%s", body)
	var noDeptEnv struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(body, &noDeptEnv))
	assert.Equal(t, 10400, noDeptEnv.Code)
	assert.Contains(t, noDeptEnv.Message, "部门不能为空", "body=%s", body)

	status, body = doAuthRequest(t, adminToken(), "POST", ts.URL+"/api/v1/users", `{"username":"bad-dept","password":"pw","realName":"坏部门","deptId":"888"}`)
	require.Equal(t, http.StatusBadRequest, status, "deptId 不存在应 400: body=%s", body)
	var badDeptEnv struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(body, &badDeptEnv))
	assert.Equal(t, 10400, badDeptEnv.Code)
	assert.Contains(t, badDeptEnv.Message, "部门不存在: 888", "body=%s", body)

	// gender=99 未知枚举 → 不 500，正常创建（枚举兜底 UNKNOWN）
	status, body = doAuthRequest(t, adminToken(), "POST", ts.URL+"/api/v1/users", `{"username":"gender-fallback","password":"pw","realName":"枚举兜底","gender":99,"deptId":"100"}`)
	require.Equal(t, http.StatusOK, status, "未知枚举值不应 500: body=%s", body)
	var okEnvelope struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(body, &okEnvelope))
	assert.Equal(t, 0, okEnvelope.Code)

	// body 携带未知额外字段 → 不 400（宽容输入）
	status, body = doAuthRequest(t, adminToken(), "POST", ts.URL+"/api/v1/users", `{"username":"unknown-field","password":"pw","realName":"未知字段","deptId":"100","foo":"bar"}`)
	require.Equal(t, http.StatusOK, status, "未知额外字段不应 400: body=%s", body)
}

// TestCreateUserBodyTooLarge 请求体超 1MB 上限（BodyLimit）：超限读取错误
// 必须收敛为固定文案 10401 envelope——不 500、不回显底层错误原文；
// 传输层无 413 关连语义：gin responseWriter 未实现 requestTooLarge 接口，该标记
// 路径不生效，连接复用交由 net/http 对未排空 body 的内置策略。
func TestCreateUserBodyTooLarge(t *testing.T) {
	ts, dsn, adminToken, _ := newTestApp(t)

	seedDept(t, dsn, 100, "总部")
	// ~1.5MB UTF-8 body，超过 1MB 上限
	big := `{"username":"body-too-large","password":"pw","realName":"` + strings.Repeat("超", 1<<19) + `"}`
	status, body := doAuthRequest(t, adminToken(), "POST", ts.URL+"/api/v1/users", big)
	require.Equal(t, http.StatusBadRequest, status, "超限 body 应收敛为 envelope 而非 500: len=%d body=%s", len(big), body)
	var envelope struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(body, &envelope), "body=%s", body)
	assert.Equal(t, 10401, envelope.Code, "超限应落 10401 段: body=%s", body)
	assert.Contains(t, envelope.Message, "1MB", "应为固定超限文案: body=%s", body)
	assert.NotContains(t, envelope.Message, "http:", "不得回显底层错误原文: body=%s", body)

	// 超限请求不落库、不污染连接：同名正常小请求仍可成功创建
	status, body = doAuthRequest(t, adminToken(), "POST", ts.URL+"/api/v1/users", `{"username":"body-too-large","password":"pw","realName":"正常","deptId":"100"}`)
	assert.Equal(t, http.StatusOK, status, "超限请求后正常请求必须仍可用: body=%s", body)
}
