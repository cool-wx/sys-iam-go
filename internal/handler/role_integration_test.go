//go:build integration

package handler_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// roleEnvelope 通用角色响应解包。
type roleEnvelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		ID        string `json:"id"`
		RoleName  string `json:"roleName"`
		RoleCode  string `json:"roleCode"`
		DataScope *int8  `json:"dataScope"`
		Status    string `json:"status"`
		Builtin   bool   `json:"builtin"`
	} `json:"data"`
}

// TestRoleCRUD 角色 CRUD 全链（ROLE-01）：create 默认值（dataScope=1/status=
// ENABLED/builtin=false）→ get → patch（roleName 变更 + builtin 键被忽略，
// WR-01 守卫）→ create builtin=true 400（WR-01）→ 角色编码重复 400 → 四连删事务
// （user_role/role_menu/role_dept/role 关联表零残留）+ 404 域消息 + pri 组 401 保护。
func TestRoleCRUD(t *testing.T) {
	ts, dsn, adminToken, _ := newTestApp(t)
	token := adminToken()

	// --- pri 组保护：无 token 401 ---
	status, body := doRequest(t, "GET", ts.URL+"/api/v1/roles", "")
	require.Equal(t, http.StatusUnauthorized, status, "无 token 必须被认证中间件拦截: body=%s", body)

	// --- create：默认值断言 ---
	status, body = doAuthRequest(t, token, "POST", ts.URL+"/api/v1/roles",
		`{"roleName":"测试角色","roleCode":"test_role_crud","remark":"CRUD 用例"}`)
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	var createEnv roleEnvelope
	require.NoError(t, json.Unmarshal(body, &createEnv))
	require.Equal(t, 0, createEnv.Code, "body=%s", body)
	roleID := createEnv.Data.ID
	roleNum, err := strconv.ParseInt(roleID, 10, 64)
	require.NoError(t, err)
	require.NotEmpty(t, roleID)
	assert.Equal(t, "测试角色", createEnv.Data.RoleName)
	assert.Equal(t, "test_role_crud", createEnv.Data.RoleCode)
	require.NotNil(t, createEnv.Data.DataScope, "dataScope 键必须存在")
	assert.Equal(t, int8(1), *createEnv.Data.DataScope, "服务端默认 dataScope=1（源 Role.create）")
	assert.Equal(t, "ENABLED", createEnv.Data.Status, "服务端默认 status=ENABLED")
	assert.False(t, createEnv.Data.Builtin, "服务端默认 builtin=false")

	// --- get ---
	status, body = doAuthRequest(t, token, "GET", ts.URL+"/api/v1/roles/"+roleID, "")
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	var getEnv roleEnvelope
	require.NoError(t, json.Unmarshal(body, &getEnv))
	assert.Equal(t, "测试角色", getEnv.Data.RoleName)

	// --- patch：roleName 变更 + builtin 键被忽略（WR-01：builtin 不可经 edit 翻转，
	// 否则先翻转 false 再 DELETE 即绕过 builtin 删除守卫）---
	status, body = doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/roles/"+roleID,
		`{"roleName":"测试角色（改）","builtin":true}`)
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	var patchEnv roleEnvelope
	require.NoError(t, json.Unmarshal(body, &patchEnv))
	assert.Equal(t, "测试角色（改）", patchEnv.Data.RoleName)
	assert.False(t, patchEnv.Data.Builtin, "builtin 键必须被忽略，不得经 edit 翻转（WR-01）")

	// --- create builtin=true 显式拒绝 400（WR-01：防 role:add 制造守卫永不可删的
	// "永生角色"）---
	status, body = doAuthRequest(t, token, "POST", ts.URL+"/api/v1/roles",
		`{"roleName":"永生角色","roleCode":"builtin_create_guard","builtin":true}`)
	require.Equal(t, http.StatusBadRequest, status, "create builtin=true 必须 400: body=%s", body)
	var builtinEnv struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(body, &builtinEnv))
	assert.Equal(t, 10400, builtinEnv.Code, "body=%s", body)
	assert.Contains(t, builtinEnv.Message, "builtin", "body=%s", body)

	// --- dataScope 域校验（IN-02）：域外值 99/-1 → 400/10401；域内 3 → 200 ---
	for _, bad := range []string{"99", "-1"} {
		status, body = doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/roles/"+roleID,
			`{"dataScope":`+bad+`}`)
		require.Equal(t, http.StatusBadRequest, status, "dataScope=%s 必须 400: body=%s", bad, body)
		var dsEnv struct {
			Code int `json:"code"`
		}
		require.NoError(t, json.Unmarshal(body, &dsEnv))
		assert.Equal(t, 10401, dsEnv.Code, "dataScope=%s 应落参数校验失败 10401: body=%s", bad, body)
	}
	status, body = doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/roles/"+roleID, `{"dataScope":3}`)
	require.Equal(t, http.StatusOK, status, "dataScope=3（域内）必须 200: body=%s", body)
	var dsOk roleEnvelope
	require.NoError(t, json.Unmarshal(body, &dsOk))
	require.NotNil(t, dsOk.Data.DataScope)
	assert.Equal(t, int8(3), *dsOk.Data.DataScope, "域内 dataScope 必须正常落库")
	// 还原档位 1（不污染后续用例）
	status, body = doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/roles/"+roleID, `{"dataScope":1}`)
	require.Equal(t, http.StatusOK, status, "body=%s", body)

	// --- 角色编码重复：400「角色编码已存在」（uk_role_code，general_ci 大小写不敏感）---
	status, body = doAuthRequest(t, token, "POST", ts.URL+"/api/v1/roles",
		`{"roleName":"重复角色","roleCode":"TEST_ROLE_CRUD"}`)
	require.Equal(t, http.StatusBadRequest, status, "角色编码重复应 400: body=%s", body)
	var dupEnv struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(body, &dupEnv))
	assert.Equal(t, 10400, dupEnv.Code)
	assert.Contains(t, dupEnv.Message, "角色编码已存在", "body=%s", body)

	// --- 四连删前置：seed user_role / role_menu / role_dept 关联行 ---
	checkDB := openCheckDB(t, dsn)
	execMust(t, checkDB,
		"INSERT INTO sys_user_role (id, user_id, username, role_id, role_name) VALUES (990501, 1, 'admin', ?, '测试角色（改）')", roleNum)
	execMust(t, checkDB,
		"INSERT INTO sys_role_menu (id, role_id, role_name, menu_id, menu_name) VALUES (990502, ?, '测试角色（改）', 990301, '菜单甲')", roleNum)
	execMust(t, checkDB,
		"INSERT INTO sys_role_dept (id, role_id, role_name, dept_id, dept_name) VALUES (990503, ?, '测试角色（改）', 990603, '四连删部门')", roleNum)

	// --- delete：四连删事务 ---
	status, body = doAuthRequest(t, token, "DELETE", ts.URL+"/api/v1/roles/"+roleID, "")
	require.Equal(t, http.StatusOK, status, "body=%s", body)

	// --- 删除后：get 404 + 域消息「角色不存在: {id}」+ 关联表零残留 ---
	status, body = doAuthRequest(t, token, "GET", ts.URL+"/api/v1/roles/"+roleID, "")
	require.Equal(t, http.StatusNotFound, status, "删除后 get 应 404: body=%s", body)
	var nfEnv struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(body, &nfEnv))
	assert.Equal(t, 10300, nfEnv.Code)
	assert.Contains(t, nfEnv.Message, "角色不存在: "+roleID, "404 消息应为域消息原文: body=%s", body)

	var userRoleN, roleMenuN, roleDeptN int64
	require.NoError(t, checkDB.Raw("SELECT COUNT(*) FROM sys_user_role WHERE role_id = ?", roleNum).Scan(&userRoleN).Error)
	require.NoError(t, checkDB.Raw("SELECT COUNT(*) FROM sys_role_menu WHERE role_id = ?", roleNum).Scan(&roleMenuN).Error)
	require.NoError(t, checkDB.Raw("SELECT COUNT(*) FROM sys_role_dept WHERE role_id = ?", roleNum).Scan(&roleDeptN).Error)
	assert.Equal(t, int64(0), userRoleN, "sys_user_role 中该 role_id 必须零残留（四连删事务）")
	assert.Equal(t, int64(0), roleMenuN, "sys_role_menu 中该 role_id 必须零残留（四连删事务）")
	assert.Equal(t, int64(0), roleDeptN, "sys_role_dept 中该 role_id 必须零残留（四连删事务，D-10 第三删）")

	// --- delete 不存在 id：404 ---
	status, body = doAuthRequest(t, token, "DELETE", ts.URL+"/api/v1/roles/99002", "")
	require.Equal(t, http.StatusNotFound, status, "body=%s", body)
}

// TestRoleBuiltinDeleteConflict builtin 角色删除硬拦截（ROLE-02/D-14）：DELETE 返回
// HTTP 409 + code=10602 + message「系统内置角色不可删除」，且该角色其后仍可 GET
// （零删除证明——前置拦截，四连删一个都不触发）。测试容器 sys_role 为空，builtin
// 角色由 SQL seed（勿动任何既有 builtin 行）。
func TestRoleBuiltinDeleteConflict(t *testing.T) {
	ts, dsn, adminToken, _ := newTestApp(t)
	token := adminToken()

	checkDB := openCheckDB(t, dsn)
	execMust(t, checkDB,
		"INSERT INTO sys_role (id, role_name, role_code, data_scope, status, builtin) VALUES (990401, '内置守卫角色', 'builtin_guard_role', 1, 1, 1)")

	status, body := doAuthRequest(t, token, "DELETE", ts.URL+"/api/v1/roles/990401", "")
	require.Equal(t, http.StatusConflict, status, "builtin 删除必须 409: body=%s", body)
	var env struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(body, &env))
	assert.Equal(t, 10602, env.Code, "业务码必须 10602: body=%s", body)
	assert.Contains(t, env.Message, "系统内置角色不可删除", "body=%s", body)

	// 零删除证明：角色其后仍可 GET
	status, body = doAuthRequest(t, token, "GET", ts.URL+"/api/v1/roles/990401", "")
	require.Equal(t, http.StatusOK, status, "builtin 角色必须仍然可 GET（零删除证明）: body=%s", body)
	var getEnv roleEnvelope
	require.NoError(t, json.Unmarshal(body, &getEnv))
	assert.Equal(t, "内置守卫角色", getEnv.Data.RoleName)
	assert.True(t, getEnv.Data.Builtin)
}

// TestRoleListFilter 角色分页条件查询（ROLE-03）：roleName 命中/未命中 + trim 语义
// （空格包裹仍命中）+ items 按 id 升序 + 无参数默认分页钳位 1/15。
func TestRoleListFilter(t *testing.T) {
	ts, _, adminToken, _ := newTestApp(t)
	token := adminToken()

	idA := mustCreate(t, token, ts.URL+"/api/v1/roles", `{"roleName":"查询甲","roleCode":"list_filter_a"}`)
	idB := mustCreate(t, token, ts.URL+"/api/v1/roles", `{"roleName":"查询乙","roleCode":"list_filter_b"}`)
	mustCreate(t, token, ts.URL+"/api/v1/roles", `{"roleName":"无关丙","roleCode":"list_filter_c"}`)

	// --- 条件命中：roleName=查询 → 甲乙两行 ---
	status, body := doAuthRequest(t, token, "GET", ts.URL+"/api/v1/roles?roleName="+url.QueryEscape("查询"), "")
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	var listEnv struct {
		Data struct {
			Items []struct {
				ID       string `json:"id"`
				RoleName string `json:"roleName"`
			} `json:"items"`
			Total int64 `json:"total"`
			Page  int   `json:"page"`
			Size  int   `json:"size"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &listEnv), "body=%s", body)
	assert.Equal(t, int64(2), listEnv.Data.Total, "roleName=查询 应命中 2 行")
	require.Len(t, listEnv.Data.Items, 2)
	assert.Equal(t, "查询甲", listEnv.Data.Items[0].RoleName)
	assert.Equal(t, "查询乙", listEnv.Data.Items[1].RoleName)
	// id 升序（源特例 orderByAsc(id)；雪花单调递增 → 创建序即 id 序）
	aNum, _ := strconv.ParseInt(idA, 10, 64)
	bNum, _ := strconv.ParseInt(idB, 10, 64)
	assert.Less(t, aNum, bNum, "fixture 前置：甲 id 应小于乙 id")
	assert.Equal(t, idA, listEnv.Data.Items[0].ID, "items 必须按 id 升序")
	assert.Equal(t, idB, listEnv.Data.Items[1].ID, "items 必须按 id 升序")

	// --- trim 语义：空格包裹的 roleName 仍命中（源 selectPage 先 trim 再 LIKE）---
	status, body = doAuthRequest(t, token, "GET", ts.URL+"/api/v1/roles?roleName="+url.QueryEscape(" 查询 "), "")
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	var trimEnv struct {
		Data struct {
			Total int64 `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &trimEnv))
	assert.Equal(t, int64(2), trimEnv.Data.Total, "空格包裹的 roleName 必须 trim 后仍命中")

	// --- 未命中：total 0 + items 空数组非 null ---
	status, body = doAuthRequest(t, token, "GET", ts.URL+"/api/v1/roles?roleName="+url.QueryEscape("绝无此角色"), "")
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	var missEnv struct {
		Data struct {
			Items []json.RawMessage `json:"items"`
			Total int64             `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &missEnv))
	assert.Equal(t, int64(0), missEnv.Data.Total)
	require.NotNil(t, missEnv.Data.Items, "空列表必须是 [] 而非 null")
	assert.Empty(t, missEnv.Data.Items)

	// --- 无参数：默认分页钳位 1/15 + 全量按 id 升序 ---
	status, body = doAuthRequest(t, token, "GET", ts.URL+"/api/v1/roles", "")
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	var allEnv struct {
		Data struct {
			Items []struct {
				ID string `json:"id"`
			} `json:"items"`
			Total int64 `json:"total"`
			Page  int   `json:"page"`
			Size  int   `json:"size"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &allEnv))
	assert.Equal(t, 1, allEnv.Data.Page, "无参数默认 page=1")
	assert.Equal(t, 15, allEnv.Data.Size, "无参数默认 size=15")
	// 03-01 起 newTestApp 内建 super_admin 种子角色（先于测试 fixture 创建——
	// 雪花单调递增，id 恒小于测试内三角色），全量基线 3→4
	assert.Equal(t, int64(4), allEnv.Data.Total)
	require.Len(t, allEnv.Data.Items, 4)
	assert.Equal(t, idA, allEnv.Data.Items[1].ID, "无过滤条件也按 id 升序（种子 super_admin 在前）")
	assert.Equal(t, idB, allEnv.Data.Items[2].ID)
}

// TestRoleAssignMenus 角色菜单分配（ROLE-03/D-13）：分配→GET 返回 menuIds→再分配
// 反映全量替换（非追加）→ menuId 缺失 400「菜单不存在: [...]」且不动存量 → 冗余列
// 落库断言。菜单 fixture 经 SQL 直插（固定 id，避开雪花）。
func TestRoleAssignMenus(t *testing.T) {
	ts, dsn, adminToken, _ := newTestApp(t)
	token := adminToken()

	checkDB := openCheckDB(t, dsn)
	execMust(t, checkDB, "INSERT INTO sys_menu (id, name, type, sort, visible, status) VALUES (990301, '菜单甲', 1, 1, 1, 1)")
	execMust(t, checkDB, "INSERT INTO sys_menu (id, name, type, sort, visible, status) VALUES (990302, '菜单乙', 1, 2, 1, 1)")
	execMust(t, checkDB, "INSERT INTO sys_menu (id, name, type, sort, visible, status) VALUES (990303, '菜单丙', 1, 3, 1, 1)")

	roleID := mustCreate(t, token, ts.URL+"/api/v1/roles", `{"roleName":"分配角色","roleCode":"assign_menus_role"}`)
	roleNum, err := strconv.ParseInt(roleID, 10, 64)
	require.NoError(t, err)

	getMenus := func() (int, []byte) {
		return doAuthRequest(t, token, "GET", ts.URL+"/api/v1/roles/"+roleID+"/menus", "")
	}
	var menusEnv struct {
		Data []string `json:"data"`
	}

	// --- 第一次分配：[甲, 乙] ---
	status, body := doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/roles/"+roleID+"/menus",
		`{"menuIds":["990301","990302"]}`)
	require.Equal(t, http.StatusOK, status, "分配应 200: body=%s", body)
	status, body = getMenus()
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	require.NoError(t, json.Unmarshal(body, &menusEnv))
	assert.ElementsMatch(t, []string{"990301", "990302"}, menusEnv.Data, "GET 应返回已分配 menuIds")

	// --- 第二次分配：[乙, 丙] —— 全量替换非追加 ---
	status, body = doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/roles/"+roleID+"/menus",
		`{"menuIds":["990302","990303"]}`)
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	status, body = getMenus()
	require.NoError(t, json.Unmarshal(body, &menusEnv))
	assert.Equal(t, []string{"990302", "990303"}, menusEnv.Data,
		"第二次分配后 GET 必须精确等于第二次提交集（全量替换，甲 被回收）")

	// --- 缺失 menuId：400 +「菜单不存在: [999]」且不动存量（前置校验）---
	status, body = doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/roles/"+roleID+"/menus",
		`{"menuIds":["990302",999]}`)
	require.Equal(t, http.StatusBadRequest, status, "缺失 menuId 应 400: body=%s", body)
	var missEnv struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(body, &missEnv))
	assert.Equal(t, 10400, missEnv.Code)
	assert.Contains(t, missEnv.Message, "菜单不存在: [", "message 应含列表前缀: body=%s", body)
	assert.Contains(t, missEnv.Message, "999", "message 应含缺失 id: body=%s", body)
	assert.NotContains(t, missEnv.Message, "990302", "message 只报缺失 id（源 missingIds 语义），已存在的 id 不得出现: body=%s", body)
	status, body = getMenus()
	require.NoError(t, json.Unmarshal(body, &menusEnv))
	assert.Equal(t, []string{"990302", "990303"}, menusEnv.Data, "校验失败不得改动存量分配")

	// --- 重复 menuId 去重（WR-02）：["甲","甲","乙"] → sys_role_menu 恰 2 行且
	// GET 回显不重复（请求体重复 ID 不得落脏行）---
	status, body = doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/roles/"+roleID+"/menus",
		`{"menuIds":["990301","990301","990302"]}`)
	require.Equal(t, http.StatusOK, status, "含重复 menuId 的分配应 200（去重后执行）: body=%s", body)
	var dupCnt int64
	require.NoError(t, checkDB.Raw(
		"SELECT COUNT(*) FROM sys_role_menu WHERE role_id = ?", roleNum,
	).Scan(&dupCnt).Error)
	assert.Equal(t, int64(2), dupCnt, "重复 menuId 必须去重，sys_role_menu 不得有重复行（WR-02）")
	status, body = getMenus()
	require.NoError(t, json.Unmarshal(body, &menusEnv))
	assert.Equal(t, []string{"990301", "990302"}, menusEnv.Data, "GET 回显不得含重复 menuId（WR-02）")

	// --- 空数组=全回收（合法语义）---
	status, body = doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/roles/"+roleID+"/menus", `{"menuIds":[]}`)
	require.Equal(t, http.StatusOK, status, "空数组回收应 200: body=%s", body)
	status, body = getMenus()
	require.NoError(t, json.Unmarshal(body, &menusEnv))
	assert.Empty(t, menusEnv.Data, "空数组回收后 GET 应为空列表")
	assert.NotNil(t, menusEnv.Data, "空列表必须序列化为 [] 而非 null")

	// --- 重新分配 [乙, 丙] 后断言 role_menu 冗余列 ---
	status, body = doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/roles/"+roleID+"/menus",
		`{"menuIds":["990302","990303"]}`)
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	var rows []struct {
		RoleID   int64
		RoleName string
		MenuID   int64
		MenuName string
	}
	require.NoError(t, checkDB.Raw(
		"SELECT role_id, role_name, menu_id, menu_name FROM sys_role_menu WHERE role_id = ? ORDER BY menu_id",
		roleNum,
	).Scan(&rows).Error)
	require.Len(t, rows, 2, "role_menu 应恰有 2 行")
	assert.Equal(t, "分配角色", rows[0].RoleName, "role_name 冗余列必须落库")
	assert.Equal(t, int64(990302), rows[0].MenuID)
	assert.Equal(t, "菜单乙", rows[0].MenuName, "menu_name 冗余列必须落库")
	assert.Equal(t, "菜单丙", rows[1].MenuName)

	// --- 分配到不存在的角色：404 域消息 ---
	status, body = doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/roles/99002/menus", `{"menuIds":[]}`)
	require.Equal(t, http.StatusNotFound, status, "body=%s", body)
}

// TestRoleAssignScopes 角色数据权限范围部门分配（PERM-03 档位 4 存储面，D-10；
// PUT/GET /api/v1/roles/:id/scopes 独立子资源端点）：分配→GET 回读→再分配反映
// 全量替换→缺失 deptId 400「部门不存在: [...]」且不动存量→空数组全回收（直查
// sys_role_dept 零行）→冗余列落库（dept_name 查库值非请求体，T-2-15）→非授权
// 角色 403/10200（RequirePerm fail-closed）→删除持有 scopes 的角色后 sys_role_dept
// 零残留（四连删直证）。部门 fixture 经 SQL 直插（固定 id，照 TestRoleAssignMenus）。
func TestRoleAssignScopes(t *testing.T) {
	ts, dsn, adminToken, _ := newTestApp(t)
	token := adminToken()

	checkDB := openCheckDB(t, dsn)
	execMust(t, checkDB,
		"INSERT INTO sys_dept (id, dept_name, sort, status, create_time, update_time) VALUES (990601, '范围部门甲', 0, 1, NOW(), NOW())")
	execMust(t, checkDB,
		"INSERT INTO sys_dept (id, dept_name, sort, status, create_time, update_time) VALUES (990602, '范围部门乙', 0, 1, NOW(), NOW())")

	roleID := mustCreate(t, token, ts.URL+"/api/v1/roles", `{"roleName":"范围角色","roleCode":"assign_scopes_role"}`)
	roleNum, err := strconv.ParseInt(roleID, 10, 64)
	require.NoError(t, err)

	getScopes := func() (int, []byte) {
		return doAuthRequest(t, token, "GET", ts.URL+"/api/v1/roles/"+roleID+"/scopes", "")
	}
	var scopesEnv struct {
		Data []string `json:"data"`
	}

	// --- 第一次分配：[甲, 乙] ---
	status, body := doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/roles/"+roleID+"/scopes",
		`{"deptIds":["990601","990602"]}`)
	require.Equal(t, http.StatusOK, status, "分配应 200: body=%s", body)
	status, body = getScopes()
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	require.NoError(t, json.Unmarshal(body, &scopesEnv))
	assert.ElementsMatch(t, []string{"990601", "990602"}, scopesEnv.Data, "GET 应回读已配置 deptIds")

	// --- 第二次分配：[甲] —— 全量替换非追加（乙 被回收）---
	status, body = doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/roles/"+roleID+"/scopes",
		`{"deptIds":["990601"]}`)
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	status, body = getScopes()
	require.NoError(t, json.Unmarshal(body, &scopesEnv))
	assert.Equal(t, []string{"990601"}, scopesEnv.Data,
		"第二次分配后 GET 必须精确等于第二次提交集（全量替换，乙 被回收）")

	// --- 缺失 deptId：400 +「部门不存在: [999]」且不动存量（前置校验）---
	status, body = doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/roles/"+roleID+"/scopes",
		`{"deptIds":["990601",999]}`)
	require.Equal(t, http.StatusBadRequest, status, "缺失 deptId 应 400: body=%s", body)
	var missEnv struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(body, &missEnv))
	assert.Equal(t, 10400, missEnv.Code)
	assert.Contains(t, missEnv.Message, "部门不存在: [", "message 应含列表前缀: body=%s", body)
	assert.Contains(t, missEnv.Message, "999", "message 应含缺失 id: body=%s", body)
	assert.NotContains(t, missEnv.Message, "990601", "message 只报缺失 id，已存在的 id 不得出现: body=%s", body)
	status, body = getScopes()
	require.NoError(t, json.Unmarshal(body, &scopesEnv))
	assert.Equal(t, []string{"990601"}, scopesEnv.Data, "校验失败不得改动存量配置")

	// --- 重复 deptId 去重（WR-02）：["甲","甲"] → sys_role_dept 恰 1 行 ---
	status, body = doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/roles/"+roleID+"/scopes",
		`{"deptIds":["990601","990601"]}`)
	require.Equal(t, http.StatusOK, status, "含重复 deptId 的分配应 200（去重后执行）: body=%s", body)
	var dupCnt int64
	require.NoError(t, checkDB.Raw(
		"SELECT COUNT(*) FROM sys_role_dept WHERE role_id = ?", roleNum,
	).Scan(&dupCnt).Error)
	assert.Equal(t, int64(1), dupCnt, "重复 deptId 必须去重，sys_role_dept 不得有重复行（WR-02）")

	// --- 空数组=全回收（合法语义）：GET 空 [] 且 sys_role_dept 直查零行 ---
	status, body = doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/roles/"+roleID+"/scopes", `{"deptIds":[]}`)
	require.Equal(t, http.StatusOK, status, "空数组回收应 200: body=%s", body)
	status, body = getScopes()
	require.NoError(t, json.Unmarshal(body, &scopesEnv))
	assert.NotNil(t, scopesEnv.Data, "空列表必须序列化为 [] 而非 null")
	assert.Empty(t, scopesEnv.Data, "空数组回收后 GET 应为空列表")
	var cnt int64
	require.NoError(t, checkDB.Raw("SELECT COUNT(*) FROM sys_role_dept WHERE role_id = ?", roleNum).Scan(&cnt).Error)
	assert.Equal(t, int64(0), cnt, "空数组全回收后 sys_role_dept 必须零行（全删重建语义）")

	// --- 重新分配 [甲, 乙] 后断言 role_dept 冗余列（dept_name 查库值，T-2-15）---
	status, body = doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/roles/"+roleID+"/scopes",
		`{"deptIds":["990601","990602"]}`)
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	var rows []struct {
		RoleID   int64
		RoleName string
		DeptID   int64
		DeptName string
	}
	require.NoError(t, checkDB.Raw(
		"SELECT role_id, role_name, dept_id, dept_name FROM sys_role_dept WHERE role_id = ? ORDER BY dept_id",
		roleNum,
	).Scan(&rows).Error)
	require.Len(t, rows, 2, "role_dept 应恰有 2 行")
	assert.Equal(t, "范围角色", rows[0].RoleName, "role_name 冗余列必须落库")
	assert.Equal(t, int64(990601), rows[0].DeptID)
	assert.Equal(t, "范围部门甲", rows[0].DeptName, "dept_name 冗余列必须落库（查库值）")
	assert.Equal(t, "范围部门乙", rows[1].DeptName)

	// --- 非授权角色访问 scopes：403/10200（无角色普通 token，RequirePerm gate）---
	seedUser(t, dsn, "scopes-no-role", testAdminHash, "无角色者", 1, false)
	uToken := loginAs(t, ts, "scopes-no-role", testAdminPass)
	status, body = doAuthRequest(t, uToken, "GET", ts.URL+"/api/v1/roles/"+roleID+"/scopes", "")
	require.Equal(t, http.StatusForbidden, status, "无权限角色必须 403: body=%s", body)
	var forbidden struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(body, &forbidden))
	assert.Equal(t, 10200, forbidden.Code, "body=%s", body)

	// --- 四连删直证：删除持有 scopes 的角色后 sys_role_dept 无残留（D-10 第三删）---
	status, body = doAuthRequest(t, token, "DELETE", ts.URL+"/api/v1/roles/"+roleID, "")
	require.Equal(t, http.StatusOK, status, "删除角色应 200: body=%s", body)
	require.NoError(t, checkDB.Raw("SELECT COUNT(*) FROM sys_role_dept WHERE role_id = ?", roleNum).Scan(&cnt).Error)
	assert.Equal(t, int64(0), cnt, "角色删除四连删后 sys_role_dept 必须零残留（D-10）")

	// --- 分配到不存在的角色：404 域消息 ---
	status, body = doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/roles/99002/scopes", `{"deptIds":[]}`)
	require.Equal(t, http.StatusNotFound, status, "body=%s", body)
}
