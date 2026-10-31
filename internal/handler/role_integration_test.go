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
// ENABLED/builtin=false）→ get → patch（roleName + builtin 显式覆写）→ 角色编码
// 重复 400 → 三连删事务（关联表零残留）+ 404 域消息 + pri 组 401 保护。
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

	// --- patch：roleName 变更 + builtin 显式覆写（源 withEdit：仅显式传入才覆写）---
	status, body = doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/roles/"+roleID,
		`{"roleName":"测试角色（改）","builtin":true}`)
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	var patchEnv roleEnvelope
	require.NoError(t, json.Unmarshal(body, &patchEnv))
	assert.Equal(t, "测试角色（改）", patchEnv.Data.RoleName)
	assert.True(t, patchEnv.Data.Builtin, "builtin 显式传入必须覆写")
	// 还原 builtin=false（后续删除走非 builtin 分支）
	status, body = doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/roles/"+roleID, `{"builtin":false}`)
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

	// --- 三连删前置：seed user_role / role_menu 关联行 ---
	checkDB := openCheckDB(t, dsn)
	execMust(t, checkDB,
		"INSERT INTO sys_user_role (id, user_id, username, role_id, role_name) VALUES (990501, 1, 'admin', ?, '测试角色（改）')", roleNum)
	execMust(t, checkDB,
		"INSERT INTO sys_role_menu (id, role_id, role_name, menu_id, menu_name) VALUES (990502, ?, '测试角色（改）', 990301, '菜单甲')", roleNum)

	// --- delete：三连删事务 ---
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

	var userRoleN, roleMenuN int64
	require.NoError(t, checkDB.Raw("SELECT COUNT(*) FROM sys_user_role WHERE role_id = ?", roleNum).Scan(&userRoleN).Error)
	require.NoError(t, checkDB.Raw("SELECT COUNT(*) FROM sys_role_menu WHERE role_id = ?", roleNum).Scan(&roleMenuN).Error)
	assert.Equal(t, int64(0), userRoleN, "sys_user_role 中该 role_id 必须零残留（三连删事务）")
	assert.Equal(t, int64(0), roleMenuN, "sys_role_menu 中该 role_id 必须零残留（三连删事务）")

	// --- delete 不存在 id：404 ---
	status, body = doAuthRequest(t, token, "DELETE", ts.URL+"/api/v1/roles/99002", "")
	require.Equal(t, http.StatusNotFound, status, "body=%s", body)
}

// TestRoleBuiltinDeleteConflict builtin 角色删除硬拦截（ROLE-02/D-14）：DELETE 返回
// HTTP 409 + code=10602 + message「系统内置角色不可删除」，且该角色其后仍可 GET
// （零删除证明——前置拦截，三连删一个都不触发）。测试容器 sys_role 为空，builtin
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
	assert.Equal(t, int64(3), allEnv.Data.Total)
	require.Len(t, allEnv.Data.Items, 3)
	assert.Equal(t, idA, allEnv.Data.Items[0].ID, "无过滤条件也按 id 升序")
	assert.Equal(t, idB, allEnv.Data.Items[1].ID)
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
