//go:build integration

package handler_test

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// treeNode 树响应解包结构（id 为字符串化雪花 ID；children 递归嵌套）。
type treeNode struct {
	ID       string     `json:"id"`
	Name     string     `json:"name"`
	ParentID *string    `json:"parentId"`
	Children []treeNode `json:"children"`
}

// envelopeID 通用解包：code/message/data.id（字符串化雪花）。
type envelopeID struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		ID string `json:"id"`
	} `json:"data"`
}

// deptTreeNode 部门树响应解包结构（部门域名称键为 deptName，与菜单域 name 区分）。
type deptTreeNode struct {
	ID       string         `json:"id"`
	DeptName string         `json:"deptName"`
	ParentID *string        `json:"parentId"`
	Children []deptTreeNode `json:"children"`
}

// mustCreate 通过带 token 的 POST 创建一行并返回字符串化 id（断言 200 + code 0）。
func mustCreate(t *testing.T, token, url, body string) string {
	t.Helper()
	status, respBody := doAuthRequest(t, token, "POST", url, body)
	require.Equal(t, http.StatusOK, status, "create 失败: body=%s", respBody)
	var env envelopeID
	require.NoError(t, json.Unmarshal(respBody, &env), "body=%s", respBody)
	require.Equal(t, 0, env.Code, "body=%s", respBody)
	require.NotEmpty(t, env.Data.ID, "data.id 必须存在")
	return env.Data.ID
}

// TestMenuCRUD 菜单 CRUD 全链：create→get→patch→delete + 父校验 400
// 分支 + 不存在 id 404 + pri 组 401 保护 + MenuType 未知值兜底 + parent_name 服务端
// 冗余（非请求体可伪造）+ 删除带子节点菜单仍成功（无子菜单引用检查）。
func TestMenuCRUD(t *testing.T) {
	ts, dsn, adminToken, _ := newTestApp(t)
	token := adminToken()

	// --- pri 组保护：无 token 访问 tree 端点 401 ---
	status, body := doRequest(t, "GET", ts.URL+"/api/v1/menus/tree", "")
	require.Equal(t, http.StatusUnauthorized, status, "无 token 必须被认证中间件拦截: body=%s", body)
	var codeEnv struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(body, &codeEnv))
	assert.Equal(t, 10100, codeEnv.Code, "未登录应落 10100: body=%s", body)

	// --- create 根菜单（type 用 name 形态）---
	rootID := mustCreate(t, token, ts.URL+"/api/v1/menus", `{"name":"系统管理","type":"DIR","sort":1,"path":"/system"}`)
	rootNum, err := strconv.ParseInt(rootID, 10, 64)
	require.NoError(t, err)

	// --- get：契约回显 ---
	status, body = doAuthRequest(t, token, "GET", ts.URL+"/api/v1/menus/"+rootID, "")
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	var getEnv struct {
		Data struct {
			ID       string  `json:"id"`
			Name     string  `json:"name"`
			Type     string  `json:"type"`
			Status   string  `json:"status"`
			Visible  bool    `json:"visible"`
			ParentID *string `json:"parentId"`
			Sort     int     `json:"sort"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &getEnv), "body=%s", body)
	assert.Equal(t, rootID, getEnv.Data.ID)
	assert.Equal(t, "系统管理", getEnv.Data.Name)
	assert.Equal(t, "DIR", getEnv.Data.Type, "type 应以 name 形态回显")
	assert.Equal(t, "ENABLED", getEnv.Data.Status, "服务端默认 status=ENABLED")
	assert.True(t, getEnv.Data.Visible, "服务端默认 visible=true")
	require.Nil(t, getEnv.Data.ParentID, "根菜单 parentId 应为 null")

	// --- create 子菜单带合法 parentId：DB parent_name 必须为服务端查库值 ---
	// 请求体携带伪造 parentName 键（DTO 无此键 → 忽略）——落库值必须来自查库
	childID := mustCreate(t, token, ts.URL+"/api/v1/menus",
		`{"name":"用户管理","type":"MENU","parentId":`+rootID+`,"parentName":"伪造的父名","sort":1}`)
	childNum, err := strconv.ParseInt(childID, 10, 64)
	require.NoError(t, err)

	checkDB := openCheckDB(t, dsn)
	var row struct {
		ParentID   *int64
		ParentName *string
	}
	require.NoError(t, checkDB.Raw(
		"SELECT parent_id, parent_name FROM sys_menu WHERE id = ?", childNum,
	).Scan(&row).Error)
	require.NotNil(t, row.ParentID)
	assert.Equal(t, rootNum, *row.ParentID)
	require.NotNil(t, row.ParentName, "合法 parentId 时 parent_name 必须冗余落库")
	assert.Equal(t, "系统管理", *row.ParentName, "parent_name 必须为服务端查库值（非请求体伪造值）")

	// --- create 带不存在的 parentId：400 +「父菜单不存在: 999」---
	status, body = doAuthRequest(t, token, "POST", ts.URL+"/api/v1/menus",
		`{"name":"孤儿菜单","type":"MENU","parentId":999}`)
	require.Equal(t, http.StatusBadRequest, status, "父不存在应 400: body=%s", body)
	var badParentEnv struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(body, &badParentEnv))
	assert.Equal(t, 10400, badParentEnv.Code)
	assert.Contains(t, badParentEnv.Message, "父菜单不存在: 999", "body=%s", body)

	// --- create type=99 未知枚举：不 500，兜底 MENU ---
	status, body = doAuthRequest(t, token, "POST", ts.URL+"/api/v1/menus",
		`{"name":"枚举兜底","type":99}`)
	require.Equal(t, http.StatusOK, status, "未知 type 不应 500: body=%s", body)

	// --- patch 子菜单名称 ---
	status, body = doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/menus/"+childID, `{"name":"用户管理（改）"}`)
	require.Equal(t, http.StatusOK, status, "patch 应 200: body=%s", body)
	var patchEnv struct {
		Data struct {
			Name string `json:"name"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &patchEnv))
	assert.Equal(t, "用户管理（改）", patchEnv.Data.Name, "patch 后回读应含新名")

	// --- patch 不存在的 id：404 ---
	status, body = doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/menus/99001", `{"name":"x"}`)
	require.Equal(t, http.StatusNotFound, status, "body=%s", body)

	// --- delete 带子节点的根菜单：仍成功（无子菜单引用检查）---
	status, body = doAuthRequest(t, token, "DELETE", ts.URL+"/api/v1/menus/"+rootID, "")
	require.Equal(t, http.StatusOK, status, "删除带子节点的菜单仍必须成功（源缺口按源保留）: body=%s", body)

	// --- 删除后 get：404 + 域消息「菜单不存在: {id}」（固定文案，code/HTTP 契约同主链）---
	status, body = doAuthRequest(t, token, "GET", ts.URL+"/api/v1/menus/"+rootID, "")
	require.Equal(t, http.StatusNotFound, status, "删除后 get 应 404: body=%s", body)
	var menuNfEnv struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(body, &menuNfEnv))
	assert.Equal(t, 10300, menuNfEnv.Code)
	assert.Contains(t, menuNfEnv.Message, "菜单不存在: "+rootID, "404 消息应为域消息原文: body=%s", body)

	// --- get 不存在的 id：404 ---
	status, body = doAuthRequest(t, token, "GET", ts.URL+"/api/v1/menus/99002", "")
	require.Equal(t, http.StatusNotFound, status, "body=%s", body)
}

// TestMenuTree 菜单树端点：根→子→孙嵌套 + parentId=999 孤儿提升为根 +
// parentId=NULL 根 + 叶子 children 空数组非 null。孤儿经 SQL 直插（API create 会
// 被父校验拦截——正确行为）。
func TestMenuTree(t *testing.T) {
	ts, dsn, adminToken, _ := newTestApp(t)
	token := adminToken()

	// fixture：根（sort 1）→ 子（sort 1）→ 孙（sort 1），孤儿（sort 99, parent 999）
	rootID := mustCreate(t, token, ts.URL+"/api/v1/menus", `{"name":"树根","type":"DIR","sort":1}`)
	childID := mustCreate(t, token, ts.URL+"/api/v1/menus", `{"name":"树子","type":"DIR","parentId":`+rootID+`,"sort":1}`)
	mustCreate(t, token, ts.URL+"/api/v1/menus", `{"name":"树孙","type":"MENU","parentId":`+childID+`,"sort":1}`)

	checkDB := openCheckDB(t, dsn)
	execMust(t, checkDB,
		"INSERT INTO sys_menu (id, parent_id, parent_name, name, type, sort, visible, status) VALUES (990101, 999, '幽灵父', '孤儿菜单', 1, 99, 1, 1)")

	status, body := doAuthRequest(t, token, "GET", ts.URL+"/api/v1/menus/tree", "")
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	var env struct {
		Data []treeNode `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &env), "body=%s", body)

	// 根层：真根（sort 1）+ 孤儿（sort 99 提升为根）——排序 sort ASC, id ASC
	require.Len(t, env.Data, 2, "真根 + 孤儿均应在根层: body=%s", body)
	assert.Equal(t, rootID, env.Data[0].ID, "根层第一项应为真根（sort 1）")
	assert.Equal(t, "孤儿菜单", env.Data[1].Name, "孤儿应提升为根（部门域同构语义）")
	require.NotNil(t, env.Data[1].ParentID, "孤儿的 parentId 键应保留原值 999")
	assert.Equal(t, "999", *env.Data[1].ParentID)

	// 孙代嵌套：根 → 子 → 孙
	require.Len(t, env.Data[0].Children, 1, "根应有且仅有一个子节点")
	child := env.Data[0].Children[0]
	assert.Equal(t, childID, child.ID)
	require.Len(t, child.Children, 1, "孙代应嵌套在子节点下")
	grandchild := child.Children[0]
	assert.Equal(t, "树孙", grandchild.Name)
	assert.NotNil(t, grandchild.Children, "叶子 children 必须是空数组 [] 而非 null")
	assert.Empty(t, grandchild.Children, "叶子 children 应为空数组")
}

// TestDeptCRUD 部门 CRUD 全链：create→get→patch→delete + 父校验
// 400「上级部门不存在」分支 + 不存在 id 404 + 删除带子节点部门仍成功（无引用检查）。
func TestDeptCRUD(t *testing.T) {
	ts, dsn, adminToken, _ := newTestApp(t)
	token := adminToken()

	// --- pri 组保护：无 token 访问 tree 端点 401 ---
	status, body := doRequest(t, "GET", ts.URL+"/api/v1/depts/tree", "")
	require.Equal(t, http.StatusUnauthorized, status, "无 token 必须被认证中间件拦截: body=%s", body)

	// --- create 根部门 ---
	rootID := mustCreate(t, token, ts.URL+"/api/v1/depts", `{"deptName":"总部","sort":1}`)
	rootNum, err := strconv.ParseInt(rootID, 10, 64)
	require.NoError(t, err)

	// --- get：契约回显 ---
	status, body = doAuthRequest(t, token, "GET", ts.URL+"/api/v1/depts/"+rootID, "")
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	var getEnv struct {
		Data struct {
			ID       string  `json:"id"`
			DeptName string  `json:"deptName"`
			Status   string  `json:"status"`
			ParentID *string `json:"parentId"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &getEnv))
	assert.Equal(t, "总部", getEnv.Data.DeptName)
	assert.Equal(t, "ENABLED", getEnv.Data.Status, "服务端默认 status=ENABLED")
	require.Nil(t, getEnv.Data.ParentID)

	// --- create 子部门带合法 parentId：DB parent_name 冗余 ---
	childID := mustCreate(t, token, ts.URL+"/api/v1/depts", `{"deptName":"研发部","parentId":`+rootID+`,"sort":1}`)
	childNum, err := strconv.ParseInt(childID, 10, 64)
	require.NoError(t, err)
	checkDB := openCheckDB(t, dsn)
	var row struct {
		ParentID   *int64
		ParentName *string
	}
	require.NoError(t, checkDB.Raw(
		"SELECT parent_id, parent_name FROM sys_dept WHERE id = ?", childNum,
	).Scan(&row).Error)
	require.NotNil(t, row.ParentID)
	assert.Equal(t, rootNum, *row.ParentID)
	require.NotNil(t, row.ParentName)
	assert.Equal(t, "总部", *row.ParentName, "parent_name 必须为服务端查库冗余值")

	// --- create 带不存在的 parentId：400 +「上级部门不存在: 888」---
	status, body = doAuthRequest(t, token, "POST", ts.URL+"/api/v1/depts", `{"deptName":"孤儿部门","parentId":888}`)
	require.Equal(t, http.StatusBadRequest, status, "父不存在应 400: body=%s", body)
	var badParentEnv struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(body, &badParentEnv))
	assert.Equal(t, 10400, badParentEnv.Code)
	assert.Contains(t, badParentEnv.Message, "上级部门不存在: 888", "body=%s", body)

	// --- patch 子部门 ---
	status, body = doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/depts/"+childID, `{"deptName":"研发中心"}`)
	require.Equal(t, http.StatusOK, status, "body=%s", body)

	// --- delete 带子节点的部门：仍成功（无引用检查）---
	status, body = doAuthRequest(t, token, "DELETE", ts.URL+"/api/v1/depts/"+rootID, "")
	require.Equal(t, http.StatusOK, status, "删除带子节点的部门仍必须成功（源缺口按源保留）: body=%s", body)

	// --- 删除后 get 404（域消息「部门不存在: {id}」）+ get 不存在 id 404 ---
	status, body = doAuthRequest(t, token, "GET", ts.URL+"/api/v1/depts/"+rootID, "")
	require.Equal(t, http.StatusNotFound, status, "删除后 get 应 404: body=%s", body)
	var deptNfEnv struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(body, &deptNfEnv))
	assert.Equal(t, 10300, deptNfEnv.Code)
	assert.Contains(t, deptNfEnv.Message, "部门不存在: "+rootID, "404 消息应为域消息原文: body=%s", body)
	status, body = doAuthRequest(t, token, "GET", ts.URL+"/api/v1/depts/99002", "")
	require.Equal(t, http.StatusNotFound, status, "body=%s", body)
}

// TestDeptTreeOrphanPromotion 部门树孤儿提升：parentId 指向
// 不存在节点的部门提升为根层（孤儿提升语义）。
func TestDeptTreeOrphanPromotion(t *testing.T) {
	ts, dsn, adminToken, _ := newTestApp(t)
	token := adminToken()

	rootID := mustCreate(t, token, ts.URL+"/api/v1/depts", `{"deptName":"树根部门","sort":1}`)
	mustCreate(t, token, ts.URL+"/api/v1/depts", `{"deptName":"树子部门","parentId":`+rootID+`,"sort":1}`)

	// 孤儿：parent_id=777（不存在），SQL 直插（API create 会被父校验拦截——正确行为）
	checkDB := openCheckDB(t, dsn)
	execMust(t, checkDB,
		"INSERT INTO sys_dept (id, parent_id, parent_name, dept_name, sort, status) VALUES (990201, 777, '幽灵部门', '孤儿部门', 99, 1)")

	status, body := doAuthRequest(t, token, "GET", ts.URL+"/api/v1/depts/tree", "")
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	var env struct {
		Data []deptTreeNode `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &env), "body=%s", body)

	// 根层：真根（sort 1）+ 孤儿（sort 99）——孤儿提升为根
	require.Len(t, env.Data, 2, "真根 + 孤儿均应在根层（孤儿提升为根）: body=%s", body)
	assert.Equal(t, rootID, env.Data[0].ID)
	assert.Equal(t, "孤儿部门", env.Data[1].DeptName)

	// 子代嵌套正确
	require.Len(t, env.Data[0].Children, 1)
	assert.Equal(t, "树子部门", env.Data[0].Children[0].DeptName)
	assert.NotNil(t, env.Data[0].Children[0].Children, "叶子 children 必须是 [] 而非 null")
	assert.Empty(t, env.Data[0].Children[0].Children)
}

// TestMenuDeptUpdateParentCycleGuard Update parentId 自指/成环 400（回归）：
// ①pid==id 自指 ②pid 为当前节点子节点 ③pid 为当前节点孙节点——均 400 且存量不动
// （若落库成环，tree.Build 对环上节点互不归根、整条子树从树查询静默消失——持久
// 脏数据，必须前置拦截）；合法移动（挂到无祖先关系的节点）仍 200。menu/dept 两域
// 同构各直证一遍。
func TestMenuDeptUpdateParentCycleGuard(t *testing.T) {
	ts, _, adminToken, _ := newTestApp(t)
	token := adminToken()

	// --- menu 域：root ← child ← grand ---
	rootID := mustCreate(t, token, ts.URL+"/api/v1/menus", `{"name":"环根","type":"DIR","sort":1}`)
	childID := mustCreate(t, token, ts.URL+"/api/v1/menus", `{"name":"环子","type":"DIR","parentId":`+rootID+`,"sort":1}`)
	grandID := mustCreate(t, token, ts.URL+"/api/v1/menus", `{"name":"环孙","type":"MENU","parentId":`+childID+`,"sort":1}`)
	otherID := mustCreate(t, token, ts.URL+"/api/v1/menus", `{"name":"无关联根","type":"DIR","sort":9}`)

	// ①自指 → 400「父菜单不能是自己」
	status, body := doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/menus/"+rootID, `{"parentId":`+rootID+`}`)
	require.Equal(t, http.StatusBadRequest, status, "自指必须 400: body=%s", body)
	var env1 struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(body, &env1))
	assert.Equal(t, 10400, env1.Code)
	assert.Contains(t, env1.Message, "父菜单不能是自己", "body=%s", body)

	// ②挂到直接子节点 → 400（成环）
	status, body = doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/menus/"+rootID, `{"parentId":`+childID+`}`)
	require.Equal(t, http.StatusBadRequest, status, "挂到子节点必须 400: body=%s", body)
	var env2 struct {
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(body, &env2))
	assert.Contains(t, env2.Message, "父菜单不能是当前菜单的子孙节点: ", "body=%s", body)

	// ③挂到孙节点 → 400（隔代成环，上溯两跳命中）
	status, body = doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/menus/"+rootID, `{"parentId":`+grandID+`}`)
	require.Equal(t, http.StatusBadRequest, status, "挂到孙节点必须 400: body=%s", body)

	// 拒绝后存量不动 + 树查询不消失：root 仍为根、child 仍在 root 下
	status, body = doAuthRequest(t, token, "GET", ts.URL+"/api/v1/menus/tree", "")
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	var treeEnv struct {
		Data []treeNode `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &treeEnv), "body=%s", body)
	found := false
	for _, r := range treeEnv.Data {
		if r.ID == rootID {
			found = true
			require.Len(t, r.Children, 1, "拒绝后 child 必须仍在 root 下")
			assert.Equal(t, childID, r.Children[0].ID)
		}
	}
	assert.True(t, found, "拒绝后 root 不得从树中消失（未成环证明）")

	// 合法移动：child 挂到无祖先关系的 other 根 → 200 + DB parent_id 联动
	status, body = doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/menus/"+childID, `{"parentId":`+otherID+`}`)
	require.Equal(t, http.StatusOK, status, "合法移动必须 200: body=%s", body)
	var moved struct {
		Data struct {
			ParentID string `json:"parentId"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &moved))
	assert.Equal(t, otherID, moved.Data.ParentID, "合法移动后 parentId 应为新父")

	// --- dept 域同构：dRoot ← dChild ← dGrand ---
	dRootID := mustCreate(t, token, ts.URL+"/api/v1/depts", `{"deptName":"环部门根","sort":1}`)
	dChildID := mustCreate(t, token, ts.URL+"/api/v1/depts", `{"deptName":"环部门子","parentId":`+dRootID+`,"sort":1}`)
	dGrandID := mustCreate(t, token, ts.URL+"/api/v1/depts", `{"deptName":"环部门孙","parentId":`+dChildID+`,"sort":1}`)

	status, body = doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/depts/"+dRootID, `{"parentId":`+dRootID+`}`)
	require.Equal(t, http.StatusBadRequest, status, "dept 自指必须 400: body=%s", body)
	var denv1 struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(body, &denv1))
	assert.Equal(t, 10400, denv1.Code)
	assert.Contains(t, denv1.Message, "上级部门不能是自己", "body=%s", body)

	status, body = doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/depts/"+dRootID, `{"parentId":`+dChildID+`}`)
	require.Equal(t, http.StatusBadRequest, status, "dept 挂到子节点必须 400: body=%s", body)

	status, body = doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/depts/"+dRootID, `{"parentId":`+dGrandID+`}`)
	require.Equal(t, http.StatusBadRequest, status, "dept 挂到孙节点必须 400: body=%s", body)

	// dept 合法移动到根（parentId=0 显式移根）仍 200
	status, body = doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/depts/"+dChildID, `{"parentId":0}`)
	require.Equal(t, http.StatusOK, status, "显式移根必须仍合法: body=%s", body)
}
