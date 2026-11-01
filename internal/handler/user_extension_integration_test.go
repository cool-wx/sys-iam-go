//go:build integration

// 用户装配域扩展集成测试（02-03 USER-01..05,07）：roles 子资源全量替换、状态停用
// 踢会话（D-09 安全增强）、重置密码踢会话、builtin 删除 409 拦截、删除清理关联、
// 两查询装配防 N+1（USER-05）——真实 MySQL+Redis 双容器。
package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/cool-wx/sys-iam-go/pkg/ids"
)

// createRole 经 POST /api/v1/roles（带 admin token）创建角色，返回 id 字符串。
func createRole(t *testing.T, ts *httptest.Server, token, name, code string) string {
	t.Helper()
	status, body := doAuthRequest(t, token, "POST", ts.URL+"/api/v1/roles",
		`{"roleName":"`+name+`","roleCode":"`+code+`"}`)
	require.Equal(t, http.StatusOK, status, "create role 失败: body=%s", body)
	var envelope struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &envelope), "body=%s", body)
	require.NotEmpty(t, envelope.Data.ID)
	return envelope.Data.ID
}

// assignRoles PUT /api/v1/users/:id/roles（roleIdsJSON 为裸 JSON 数组元素串，
// 如 `"1", "2"` 或空串=全回收）。
func assignRoles(t *testing.T, ts *httptest.Server, token, userID, roleIDsJSON string) (int, []byte) {
	t.Helper()
	return doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/users/"+userID+"/roles",
		`{"roleIds":[`+roleIDsJSON+`]}`)
}

// rolesRef 角色引用解包形状（GET roles 响应与用户列表 roles 元素共用）。
type rolesRef struct {
	RoleID   string `json:"roleId"`
	RoleName string `json:"roleName"`
}

// getUserRoles GET /api/v1/users/:id/roles 解包（断言 200 + 返回角色引用集与原始
// body——body 供空集合序列化形状断言（[] 非 null））。
func getUserRoles(t *testing.T, ts *httptest.Server, token, userID string) ([]rolesRef, []byte) {
	t.Helper()
	status, body := doAuthRequest(t, token, "GET", ts.URL+"/api/v1/users/"+userID+"/roles", "")
	require.Equal(t, http.StatusOK, status, "GET roles 失败: body=%s", body)
	var envelope struct {
		Data []rolesRef `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &envelope), "body=%s", body)
	return envelope.Data, body
}

// grantUserPerm 为运行时用户授权单条 perm（03-03 全量收口适配）：DB 直插 perm 菜单
// + seedRoleWithScope 角色，再走 API 授权链（PUT /roles/:id/menus + PUT /users/:id/roles）
// 同步 enforcer——g/p 行运行时写入使被测用户当前会话下一请求即持 perm。boot 对账只
// 覆盖 newTestApp 种子三件套；本 helper 服务 app.New 之后经 API 创建的用户（03-01
// 曾以"改走未挂 gate 端点"绕行，03-03 全量收口后无未挂 gate 的管理端点，探活必须
// 修种子授权——prohibition：不拆 gate）。
func grantUserPerm(t *testing.T, ts *httptest.Server, token, dsn string, userID int64, perm, roleCode string) {
	t.Helper()
	db := openCheckDB(t, dsn)
	menuID := int64(ids.NewID())
	execMust(t, db,
		"INSERT INTO sys_menu (id, name, permission, create_time, update_time) VALUES (?, '探活授权菜单', ?, NOW(), NOW())",
		menuID, perm,
	)
	roleID := seedRoleWithScope(t, dsn, "探活角色", roleCode, false, 5)
	roleIDStr := strconv.FormatInt(roleID, 10)
	status, body := doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/roles/"+roleIDStr+"/menus",
		`{"menuIds":["`+strconv.FormatInt(menuID, 10)+`"]}`)
	require.Equal(t, http.StatusOK, status, "授菜单必须成功: body=%s", body)
	status, body = doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/users/"+strconv.FormatInt(userID, 10)+"/roles",
		`{"roleIds":["`+roleIDStr+`"]}`)
	require.Equal(t, http.StatusOK, status, "授角色必须成功: body=%s", body)
}

// roleIDSet 提取 roleId 集合（无序比较用）。
func roleIDSet(refs []rolesRef) map[string]string {
	m := make(map[string]string, len(refs))
	for _, r := range refs {
		m[r.RoleID] = r.RoleName
	}
	return m
}

// userRoleCount 查 sys_user_role 中某用户的关联行数（DB 行级断言）。
func userRoleCount(t *testing.T, db *gorm.DB, userID int64) int64 {
	t.Helper()
	var cnt int64
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM sys_user_role WHERE user_id = ?", userID).Scan(&cnt).Error)
	return cnt
}

// TestUserRolesAssignReplace 角色分配全量替换语义（USER-04/D-12）：分配两角色 →
// GET 返回 2 角色 → 再分配 1 角色 → GET 反映全量替换（重分配不叠加）→ PUT 空数组 →
// GET 空 []。DB 行断言：sys_user_role.username 列存 realName（源命名与内容不符
// 原样移植）、role_name 冗余为角色名。
func TestUserRolesAssignReplace(t *testing.T) {
	ts, dsn, adminToken, _ := newTestApp(t)
	token := adminToken()
	seedDept(t, dsn, 100, "总部")

	uidStr := createRawUser(t, ts, token, `{"username":"assign-replace","password":"pw-123456","realName":"装配名","deptId":"100"}`)
	uid, err := strconv.ParseInt(uidStr, 10, 64)
	require.NoError(t, err)
	roleA := createRole(t, ts, token, "角色甲", "role_a")
	roleB := createRole(t, ts, token, "角色乙", "role_b")

	// 分配 [A, B] → GET 返回 2 角色
	status, body := assignRoles(t, ts, token, uidStr, `"`+roleA+`", "`+roleB+`"`)
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	refs, getBody := getUserRoles(t, ts, token, uidStr)
	require.Len(t, refs, 2)
	got := roleIDSet(refs)
	assert.Equal(t, "角色甲", got[roleA])
	assert.Equal(t, "角色乙", got[roleB])

	// DB 行：username 列 = realName（「装配名」非空白优先，源命名错位原样移植）、
	// role_name 冗余
	checkDB := openCheckDB(t, dsn)
	type urRow struct {
		Username string
		RoleName string
		RoleID   int64
	}
	var rows []urRow
	require.NoError(t, checkDB.Raw(
		"SELECT username, role_name, role_id FROM sys_user_role WHERE user_id = ? ORDER BY role_id", uid,
	).Scan(&rows).Error)
	require.Len(t, rows, 2)
	for _, r := range rows {
		assert.Equal(t, "装配名", r.Username, "sys_user_role.username 列必须存 realName（源命名错位原样移植）")
	}
	require.Equal(t, int64(2), userRoleCount(t, checkDB, uid))

	// 再分配 [B] → 全量替换（A 被删），GET 只剩 B
	status, body = assignRoles(t, ts, token, uidStr, `"`+roleB+`"`)
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	refs, getBody = getUserRoles(t, ts, token, uidStr)
	require.Len(t, refs, 1, "全量替换语义：旧关联必须被删除: body=%s", body)
	assert.Equal(t, roleB, refs[0].RoleID)
	require.Equal(t, int64(1), userRoleCount(t, checkDB, uid))

	// PUT 空数组 → 全回收，GET 返回 []（非 null）
	status, body = assignRoles(t, ts, token, uidStr, ``)
	require.Equal(t, http.StatusOK, status, "空数组=全回收必须合法: body=%s", body)
	refs, getBody = getUserRoles(t, ts, token, uidStr)
	assert.NotNil(t, refs)
	assert.Empty(t, refs)
	assert.Contains(t, string(getBody), `"data":[]`, "空角色列表必须序列化为 [] 而非 null")
	require.Equal(t, int64(0), userRoleCount(t, checkDB, uid))
}

// TestUserRolesInvalidRole 分配含不存在 roleId：400 + 消息精确含
// 「角色不存在: [逗号拼接]」式串（缺失集合式，格式照源）；校验前置不动存量。
func TestUserRolesInvalidRole(t *testing.T) {
	ts, dsn, adminToken, _ := newTestApp(t)
	token := adminToken()
	seedDept(t, dsn, 100, "总部")

	uidStr := createRawUser(t, ts, token, `{"username":"invalid-role","password":"pw-123456","realName":"坏引用","deptId":"100"}`)
	roleA := createRole(t, ts, token, "有效角色", "role_valid")

	status, body := assignRoles(t, ts, token, uidStr, `"`+roleA+`", "990003", "990004"`)
	require.Equal(t, http.StatusBadRequest, status, "含不存在角色应 400: body=%s", body)
	var envelope struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(body, &envelope))
	assert.Equal(t, 10400, envelope.Code)
	assert.Equal(t, "角色不存在: [990003, 990004]", envelope.Message, "缺失集合消息须精确: body=%s", body)

	// 校验前置：存量不动
	refs, _ := getUserRoles(t, ts, token, uidStr)
	assert.Empty(t, refs)
}

// TestUserStatusDisableKickout 停用即踢存量会话三段直证（USER-02/T-2-13/D-09）：
// ①停用后旧 token 访问 401 ②停用用户登录被拒 10502 ③重新启用后可登录。
func TestUserStatusDisableKickout(t *testing.T) {
	ts, dsn, adminToken, _ := newTestApp(t)
	token := adminToken()
	seedDept(t, dsn, 100, "总部")

	uidStr := createRawUser(t, ts, token, `{"username":"status-user","password":"pw-123456","realName":"停用者","deptId":"100"}`)
	uid, err := strconv.ParseInt(uidStr, 10, 64)
	require.NoError(t, err)

	// 03-03 全量收口后详情端点同样挂 requirePerm gate——探活用户必须持
	// system:user:query（API 授权链运行时同步 enforcer）
	grantUserPerm(t, ts, token, dsn, uid, "system:user:query", "probe_status_role")

	// 目标用户登录拿 token 并确认可用（认证可用性断言语义不变）
	tokenU := loginAs(t, ts, "status-user", "pw-123456")
	status, body := doAuthRequest(t, tokenU, "GET", ts.URL+"/api/v1/users/"+uidStr, "")
	require.Equal(t, http.StatusOK, status, "停用前 token 应可用: body=%s", body)

	// admin 停用
	status, body = doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/users/"+uidStr+"/status", `{"status":"DISABLED"}`)
	require.Equal(t, http.StatusOK, status, "停用应 200: body=%s", body)

	// ①旧 token 即刻失效
	status, body = doAuthRequest(t, tokenU, "GET", ts.URL+"/api/v1/users/"+uidStr, "")
	require.Equal(t, http.StatusUnauthorized, status, "停用后旧 token 必须 401: body=%s", body)

	// ②停用用户登录被拒 10502
	status, body = doRequest(t, "POST", ts.URL+"/api/v1/auth/login", `{"username":"status-user","password":"pw-123456"}`)
	require.Equal(t, http.StatusUnauthorized, status, "停用用户登录应 401: body=%s", body)
	var envelope struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(body, &envelope))
	assert.Equal(t, 10502, envelope.Code, "停用登录 code 必须 10502: body=%s", body)

	// ③重新启用后可登录
	status, body = doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/users/"+uidStr+"/status", `{"status":"ENABLED"}`)
	require.Equal(t, http.StatusOK, status, "启用应 200: body=%s", body)
	tokenU2 := loginAs(t, ts, "status-user", "pw-123456")
	status, body = doAuthRequest(t, tokenU2, "GET", ts.URL+"/api/v1/users/"+uidStr, "")
	assert.Equal(t, http.StatusOK, status, "启用后新登录必须可用: body=%s", body)
}

// TestResetPasswordFlow 重置密码流（USER-03/T-2-14/D-09）：登录 tokenA → admin 重置
// （无旧密码校验）→ tokenA 即刻 401 → 用 123456 登录成功且 firstLogin=true → 旧密码
// 已被覆写失效。
func TestResetPasswordFlow(t *testing.T) {
	ts, dsn, adminToken, _ := newTestApp(t)
	token := adminToken()
	seedDept(t, dsn, 100, "总部")

	uidStr := createRawUser(t, ts, token, `{"username":"reset-user","password":"pw-123456","realName":"重置者","deptId":"100"}`)
	uid, err := strconv.ParseInt(uidStr, 10, 64)
	require.NoError(t, err)

	// 03-03 全量收口后探活用户必须持 system:user:query（API 授权链，同上）
	grantUserPerm(t, ts, token, dsn, uid, "system:user:query", "probe_reset_role")

	tokenA := loginAs(t, ts, "reset-user", "pw-123456")
	status, body := doAuthRequest(t, tokenA, "GET", ts.URL+"/api/v1/users/"+uidStr, "")
	require.Equal(t, http.StatusOK, status, "重置前 token 应可用: body=%s", body)

	// admin 重置（无请求体）
	status, body = doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/users/"+uidStr+"/reset-password", "")
	require.Equal(t, http.StatusOK, status, "重置应 200: body=%s", body)

	// tokenA 即刻失效
	status, body = doAuthRequest(t, tokenA, "GET", ts.URL+"/api/v1/users/"+uidStr, "")
	require.Equal(t, http.StatusUnauthorized, status, "重置后旧 token 必须 401: body=%s", body)

	// 123456 登录成功且 firstLogin=true
	status, body = doRequest(t, "POST", ts.URL+"/api/v1/auth/login", `{"username":"reset-user","password":"123456"}`)
	require.Equal(t, http.StatusOK, status, "123456 登录应成功: body=%s", body)
	var loginEnv struct {
		Data struct {
			User struct {
				ID         string `json:"id"`
				FirstLogin *bool  `json:"firstLogin"`
			} `json:"user"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &loginEnv))
	require.NotNil(t, loginEnv.Data.User.FirstLogin, "firstLogin 键必须存在")
	assert.True(t, *loginEnv.Data.User.FirstLogin, "重置后 firstLogin 必须 true")

	// DB 行：password 列为标准 $2a$ bcrypt 哈希（绝不明文）
	checkDB := openCheckDB(t, dsn)
	var pw string
	require.NoError(t, checkDB.Raw("SELECT password FROM sys_user WHERE id = ?", uid).Scan(&pw).Error)
	require.Len(t, pw, 60, "bcrypt 哈希必须 60 字符")
	assert.Contains(t, pw, "$2a$", "password 列必须为 bcrypt 哈希")
	assert.NotEqual(t, "123456", pw, "密码列绝不明文")

	// 旧密码已被覆写失效
	status, body = doRequest(t, "POST", ts.URL+"/api/v1/auth/login", `{"username":"reset-user","password":"pw-123456"}`)
	require.Equal(t, http.StatusUnauthorized, status, "旧密码必须已失效: body=%s", body)
	var oldPwEnv struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(body, &oldPwEnv))
	assert.Equal(t, 10501, oldPwEnv.Code)
}

// TestDeleteBuiltinUser409 builtin 用户删除硬拦截（USER-07/D-14）：DELETE builtin=true
// 用户 → HTTP 409 + code=10601「系统内置用户不可删除」（零删除调用前置拦截）→
// 用户仍存在。
func TestDeleteBuiltinUser409(t *testing.T) {
	ts, dsn, adminToken, _ := newTestApp(t)
	token := adminToken()

	builtinID := seedUser(t, dsn, "builtin-user", testAdminHash, "内置者", 1, true)
	uidStr := strconv.FormatInt(builtinID, 10)

	status, body := doAuthRequest(t, token, "DELETE", ts.URL+"/api/v1/users/"+uidStr, "")
	require.Equal(t, http.StatusConflict, status, "builtin 删除必须 409: body=%s", body)
	var envelope struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(body, &envelope))
	assert.Equal(t, 10601, envelope.Code, "body=%s", body)
	assert.Equal(t, "系统内置用户不可删除", envelope.Message)

	// 零删除证明：用户仍在
	status, body = doAuthRequest(t, token, "GET", ts.URL+"/api/v1/users/"+uidStr, "")
	require.Equal(t, http.StatusOK, status, "builtin 用户必须仍存在: body=%s", body)
	var getEnv struct {
		Data struct {
			Builtin bool `json:"builtin"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &getEnv))
	assert.True(t, getEnv.Data.Builtin)
}

// TestDeleteUserCleansRelations 普通用户删除清理关联：分配角色后 DELETE → 200、
// sys_user_role 该 user_id 行数 0、GET 404 域消息（先删关联再删用户，源顺序）。
func TestDeleteUserCleansRelations(t *testing.T) {
	ts, dsn, adminToken, _ := newTestApp(t)
	token := adminToken()
	seedDept(t, dsn, 100, "总部")

	uidStr := createRawUser(t, ts, token, `{"username":"delete-me","password":"pw-123456","realName":"待删者","deptId":"100"}`)
	uid, err := strconv.ParseInt(uidStr, 10, 64)
	require.NoError(t, err)
	roleA := createRole(t, ts, token, "待删角色", "role_del")
	status, body := assignRoles(t, ts, token, uidStr, `"`+roleA+`"`)
	require.Equal(t, http.StatusOK, status, "body=%s", body)

	checkDB := openCheckDB(t, dsn)
	require.Equal(t, int64(1), userRoleCount(t, checkDB, uid), "删除前应有 1 行关联")

	status, body = doAuthRequest(t, token, "DELETE", ts.URL+"/api/v1/users/"+uidStr, "")
	require.Equal(t, http.StatusOK, status, "普通用户删除应 200: body=%s", body)

	// 关联零残留 + 用户行已删
	require.Equal(t, int64(0), userRoleCount(t, checkDB, uid), "sys_user_role 关联必须清零")
	var userCnt int64
	require.NoError(t, checkDB.Raw("SELECT COUNT(*) FROM sys_user WHERE id = ?", uid).Scan(&userCnt).Error)
	assert.Equal(t, int64(0), userCnt, "用户行必须已删")

	status, body = doAuthRequest(t, token, "GET", ts.URL+"/api/v1/users/"+uidStr, "")
	require.Equal(t, http.StatusNotFound, status, "删除后 GET 应 404: body=%s", body)
	var nfEnv struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(body, &nfEnv))
	assert.Equal(t, 10300, nfEnv.Code)
	assert.Equal(t, "用户不存在: "+uidStr, nfEnv.Message)
}

// TestUserListDetailRolesAssembly 两查询装配（USER-05）：3 用户各配不同角色 →
// list 与 get 的 roles 形状正确、互不串扰；无角色用户 roles 为 [] 非 null。
func TestUserListDetailRolesAssembly(t *testing.T) {
	ts, dsn, adminToken, _ := newTestApp(t)
	token := adminToken()
	seedDept(t, dsn, 100, "总部")

	u1 := createRawUser(t, ts, token, `{"username":"asm-u1","password":"pw-123456","realName":"装配一","deptId":"100"}`)
	u2 := createRawUser(t, ts, token, `{"username":"asm-u2","password":"pw-123456","realName":"装配二","deptId":"100"}`)
	u3 := createRawUser(t, ts, token, `{"username":"asm-u3","password":"pw-123456","realName":"装配三","deptId":"100"}`)

	roleA := createRole(t, ts, token, "装配角色甲", "role_asm_a")
	roleB := createRole(t, ts, token, "装配角色乙", "role_asm_b")
	roleC := createRole(t, ts, token, "装配角色丙", "role_asm_c")

	status, body := assignRoles(t, ts, token, u1, `"`+roleA+`"`)
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	status, body = assignRoles(t, ts, token, u2, `"`+roleB+`", "`+roleC+`"`)
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	// u3 从不分配——空角色

	// --- list 装配 ---
	status, body = doAuthRequest(t, token, "GET", ts.URL+"/api/v1/users?page=1&size=15", "")
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	var listEnv struct {
		Data struct {
			Items []struct {
				Username string     `json:"username"`
				Roles    []rolesRef `json:"roles"`
			} `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &listEnv), "body=%s", body)

	byName := make(map[string][]rolesRef, len(listEnv.Data.Items))
	for _, it := range listEnv.Data.Items {
		byName[it.Username] = it.Roles
	}
	require.Contains(t, byName, "asm-u1")
	require.Contains(t, byName, "asm-u2")
	require.Contains(t, byName, "asm-u3")

	got1 := roleIDSet(byName["asm-u1"])
	require.Len(t, got1, 1, "u1 应恰有 1 角色: %v", byName["asm-u1"])
	assert.Equal(t, "装配角色甲", got1[roleA])

	got2 := roleIDSet(byName["asm-u2"])
	require.Len(t, got2, 2, "u2 应恰有 2 角色: %v", byName["asm-u2"])
	assert.Equal(t, "装配角色乙", got2[roleB])
	assert.Equal(t, "装配角色丙", got2[roleC])

	assert.NotNil(t, byName["asm-u3"], "u3 roles 键必须存在（[] 非 null）")
	assert.Empty(t, byName["asm-u3"], "u3 无角色")
	assert.NotContains(t, got1, roleB, "roles 不得串扰（u1 不得含 u2 的角色）")
	assert.NotContains(t, got2, roleA, "roles 不得串扰（u2 不得含 u1 的角色）")

	// --- 详情装配 ---
	for _, tc := range []struct {
		uid  string
		want map[string]string
	}{
		{u1, map[string]string{roleA: "装配角色甲"}},
		{u2, map[string]string{roleB: "装配角色乙", roleC: "装配角色丙"}},
	} {
		status, body = doAuthRequest(t, token, "GET", ts.URL+"/api/v1/users/"+tc.uid, "")
		require.Equal(t, http.StatusOK, status, "body=%s", body)
		var getEnv struct {
			Data struct {
				ID    string     `json:"id"`
				Roles []rolesRef `json:"roles"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(body, &getEnv), "body=%s", body)
		assert.Equal(t, tc.uid, getEnv.Data.ID)
		assert.Equal(t, tc.want, roleIDSet(getEnv.Data.Roles), "uid=%s 详情 roles 形状", tc.uid)
	}

	// u3 详情 roles 为 []
	status, body = doAuthRequest(t, token, "GET", ts.URL+"/api/v1/users/"+u3, "")
	require.Equal(t, http.StatusOK, status)
	assert.Contains(t, string(body), `"roles":[]`, "无角色用户详情 roles 必须 [] 非 null")
}

// TestDeleteUserKicksSessions 删除即踢存量会话（USER-07/D-09，CR-01 修复回归）：
// 目标用户登录拿 token 确认可用 → admin 删除 → 旧 token 即刻 401/10100。认证中间件
// 仅校验 Redis 会话存在性、不回查 DB——删除后不踢则被删账号旧 token 在整个会话 TTL
// 内继续完全可用（删除形同未遂），删除必须与 UpdateStatus/ResetPassword 同语义踢会话。
func TestDeleteUserKicksSessions(t *testing.T) {
	ts, dsn, adminToken, _ := newTestApp(t)
	token := adminToken()
	seedDept(t, dsn, 100, "总部")

	uidStr := createRawUser(t, ts, token, `{"username":"delete-kick","password":"pw-123456","realName":"删踢者","deptId":"100"}`)
	uid, err := strconv.ParseInt(uidStr, 10, 64)
	require.NoError(t, err)

	// 03-03 全量收口后探活用户必须持 system:user:query（API 授权链，同上——
	// 无角色普通用户走任何管理端点都会被授权层 403 污染认证语义）
	grantUserPerm(t, ts, token, dsn, uid, "system:user:query", "probe_delkick_role")

	tokenU := loginAs(t, ts, "delete-kick", "pw-123456")
	status, body := doAuthRequest(t, tokenU, "GET", ts.URL+"/api/v1/users/"+uidStr, "")
	require.Equal(t, http.StatusOK, status, "删除前 token 应可用: body=%s", body)

	status, body = doAuthRequest(t, token, "DELETE", ts.URL+"/api/v1/users/"+uidStr, "")
	require.Equal(t, http.StatusOK, status, "删除应 200: body=%s", body)

	// 删除后旧 token 即刻失效（不回查 DB 的认证链依赖删除即踢）
	status, body = doAuthRequest(t, tokenU, "GET", ts.URL+"/api/v1/users/"+uidStr, "")
	require.Equal(t, http.StatusUnauthorized, status, "删除后旧 token 必须 401: body=%s", body)
	var envelope struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(body, &envelope))
	assert.Equal(t, 10100, envelope.Code, "删除后旧 token 应落未登录 10100: body=%s", body)
}

// TestUserStatusInvalidEnumRejected 状态变更端点非法枚举值 400（WR-03 回归）：
// 合法停用后，拼写错误的启用值（"ENABLD"）必须 400/10401（UnmarshalJSON 收紧报错
// → 绑定层译参数校验失败），状态不得被静默兜底为 ENABLED 反转——登录仍 10502。
// api-keys status 端点同型直证一行。
func TestUserStatusInvalidEnumRejected(t *testing.T) {
	ts, dsn, adminToken, adminID := newTestApp(t)
	token := adminToken()
	seedDept(t, dsn, 100, "总部")

	uidStr := createRawUser(t, ts, token, `{"username":"enum-guard","password":"pw-123456","realName":"枚举守卫","deptId":"100"}`)

	// 合法停用
	status, body := doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/users/"+uidStr+"/status", `{"status":"DISABLED"}`)
	require.Equal(t, http.StatusOK, status, "合法停用应 200: body=%s", body)

	// 拼写错误的启用 → 400，绝不静默执行启用
	status, body = doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/users/"+uidStr+"/status", `{"status":"ENABLD"}`)
	require.Equal(t, http.StatusBadRequest, status, "拼写错误枚举值必须 400: body=%s", body)
	var badEnum struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(body, &badEnum))
	assert.Equal(t, 10401, badEnum.Code, "UnmarshalJSON 错误应译 10401 参数校验失败: body=%s", body)

	// 停用语义未被反转：登录仍 10502（若静默兜底 ENABLED，此处会 200 登录成功）
	status, body = doRequest(t, "POST", ts.URL+"/api/v1/auth/login", `{"username":"enum-guard","password":"pw-123456"}`)
	require.Equal(t, http.StatusUnauthorized, status, "拼写错误启用不得反转停用态: body=%s", body)
	var loginEnv struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(body, &loginEnv))
	assert.Equal(t, 10502, loginEnv.Code, "body=%s", body)

	// api-keys status 端点同型：非法枚举 400
	status, body = doAuthRequest(t, token, "POST", ts.URL+"/api/v1/api-keys", `{"userId":"`+strconv.FormatInt(adminID, 10)+`"}`)
	require.Equal(t, http.StatusOK, status, "创建 api-key 应 200: body=%s", body)
	var created struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &created))
	status, body = doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/api-keys/"+created.Data.ID+"/status", `{"status":"DISABLE"}`)
	require.Equal(t, http.StatusBadRequest, status, "api-key status 非法枚举必须 400: body=%s", body)
	var apiKeyBad struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(body, &apiKeyBad))
	assert.Equal(t, 10401, apiKeyBad.Code, "body=%s", body)
}
