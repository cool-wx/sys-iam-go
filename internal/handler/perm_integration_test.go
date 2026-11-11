//go:build integration

// Casbin enforcer 装配与 requirePerm 即时生效集成测试（真实 MySQL+Redis 双容器，
// 复用 newTestApp 基建）。覆盖：boot 对账 g 行导入 + matcher 豁免独立于 p 行、
// 无角色 403/10200、无 token 401、分配/回收同 token 下一请求生效
// （即时生效）、双轨落库、级联清理。
package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cool-wx/sys-iam-go/internal/repository"
	"github.com/cool-wx/sys-iam-go/pkg/ids"
)

// TestPermEnforcerBootSmoke enforcer 冷启动冒烟三断言（Task 1 tracer 切片）：
//
//	① admin（super_admin）GET /api/v1/users 返回 200——同时直证 boot 对账 g 行
//	   导入与 matcher 豁免独立于 p 行（此时 casbin_rule 只有 g 行无 p 行，200 依然
//	   成立）；② 新 seed 无角色用户登录拿 token → 403 且 body code==10200；
//	③ 无 token → 401（Auth 中间件先挡）。
func TestPermEnforcerBootSmoke(t *testing.T) {
	ts, dsn, adminToken, _ := newTestApp(t)

	// ① admin（super_admin 豁免走 enforcer g 层）→ 200
	status, body := doAuthRequest(t, adminToken(), "GET", ts.URL+"/api/v1/users", "")
	require.Equal(t, http.StatusOK, status, "admin(super_admin) 必须放行: body=%s", body)

	// 直查 casbin_rule：boot 对账已导入 g 行（>=1）且 p 行为 0（豁免独立于 p 行）
	db := openCheckDB(t, dsn)
	var counts struct {
		GRows *int64
		PRows *int64
	}
	require.NoError(t, db.Raw(
		"SELECT SUM(ptype = 'g') AS g_rows, SUM(ptype = 'p') AS p_rows FROM casbin_rule",
	).Scan(&counts).Error)
	require.NotNil(t, counts.GRows, "casbin_rule 必须非空（boot 对账导入 g 行）")
	assert.GreaterOrEqual(t, *counts.GRows, int64(1), "boot 对账必须导入 g 行(admin→super_admin)")
	if counts.PRows != nil {
		assert.Equal(t, int64(0), *counts.PRows, "无 sys_role_menu 数据时 p 行必须为 0（matcher 豁免独立于 p 行）")
	}

	// ② 无角色用户 → 403/10200（无 g 行 → matcher 首位豁免不命中、无 p 行匹配）
	seedUser(t, dsn, "perm-no-role", testAdminHash, "无角色者", 1, false)
	uToken := loginAs(t, ts, "perm-no-role", testAdminPass)
	status, body = doAuthRequest(t, uToken, "GET", ts.URL+"/api/v1/users", "")
	require.Equal(t, http.StatusForbidden, status, "无 p 策略角色用户必须 403: body=%s", body)
	var forbidden struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(body, &forbidden))
	assert.Equal(t, 10200, forbidden.Code, "body=%s", body)

	// ③ 无 token → 401（Auth 中间件先拦，requirePerm 不可达）
	status, body = doRequest(t, "GET", ts.URL+"/api/v1/users", "")
	require.Equal(t, http.StatusUnauthorized, status, "无 token 必须 401: body=%s", body)
	var unauth struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(body, &unauth))
	assert.Equal(t, 10100, unauth.Code)
}

// permFixture 即时生效三测试共用种子：菜单 M（permission=system:user:list）+
// 非内置角色 R + 用户 U，返回三者 ID。
func permFixture(t *testing.T, dsn string, roleCode, username string) (menuID, roleID, userID int64) {
	t.Helper()
	db := openCheckDB(t, dsn)
	menuID = int64(ids.NewID())
	execMust(t, db,
		"INSERT INTO sys_menu (id, name, permission, create_time, update_time) VALUES (?, '用户列表', 'system:user:list', NOW(), NOW())",
		menuID,
	)
	roleID = seedRole(t, dsn, "权限测试角色", roleCode, false)
	userID = seedUser(t, dsn, username, testAdminHash, "权限测试者", 1, false)
	return menuID, roleID, userID
}

// TestPermHotReload 即时生效四步链（验收核心）：
// (a) admin 分配角色 → g 行已在 AssignRoles 同步写入 → U 登录拿 token → 403
// （R 无 p 策略）；(b) admin PUT /roles/{R}/menus [M] → **同一 U token 不重登录**
// 再 GET → 200；(c) 置空 menuIds（全回收）→ 同一 token 再请求 → 403；
// (d) 双轨断言：(b) 后 casbin_rule 存在 (p, role_code, perm) 行且 sys_role_menu
// 行存在；(c) 后两轨同步消失。
func TestPermHotReload(t *testing.T) {
	ts, dsn, adminToken, _ := newTestApp(t)
	db := openCheckDB(t, dsn)

	const roleCode = "perm_hot_reload_role"
	menuID, roleID, userID := permFixture(t, dsn, roleCode, "perm-hot-reload")
	roleIDStr := strconv.FormatInt(roleID, 10)
	userIDStr := strconv.FormatInt(userID, 10)
	menuIDStr := strconv.FormatInt(menuID, 10)

	// (a) 分配角色（g 行即时同步）→ U 登录 → 无 p 策略 403
	status, body := doAuthRequest(t, adminToken(), "PUT", ts.URL+"/api/v1/users/"+userIDStr+"/roles",
		`{"roleIds":["`+roleIDStr+`"]}`)
	require.Equal(t, http.StatusOK, status, "分配角色必须成功: body=%s", body)
	uToken := loginAs(t, ts, "perm-hot-reload", testAdminPass)
	status, body = doAuthRequest(t, uToken, "GET", ts.URL+"/api/v1/users", "")
	require.Equal(t, http.StatusForbidden, status, "R 无 p 策略必须 403: body=%s", body)

	// (b) admin 分配含目标 perm 串的菜单 → 同一 token 下一请求 200（不重登录）
	status, body = doAuthRequest(t, adminToken(), "PUT", ts.URL+"/api/v1/roles/"+roleIDStr+"/menus",
		`{"menuIds":["`+menuIDStr+`"]}`)
	require.Equal(t, http.StatusOK, status, "分配菜单必须成功: body=%s", body)
	status, body = doAuthRequest(t, uToken, "GET", ts.URL+"/api/v1/users", "")
	require.Equal(t, http.StatusOK, status, "分配后同一 token 下一请求必须 200（即时生效直证）: body=%s", body)

	// (d1) 双轨落库：casbin_rule p 行 + sys_role_menu 行同写
	var count struct {
		PRows  int64
		RMRows int64
	}
	require.NoError(t, db.Raw(
		"SELECT (SELECT COUNT(*) FROM casbin_rule WHERE ptype='p' AND v0=? AND v1='system:user:list') AS p_rows, "+
			"(SELECT COUNT(*) FROM sys_role_menu WHERE role_id=? AND menu_id=?) AS rm_rows",
		roleCode, roleID, menuID,
	).Scan(&count).Error)
	assert.Equal(t, int64(1), count.PRows, "casbin_rule 必须存在 (p, role_code, perm) 行")
	assert.Equal(t, int64(1), count.RMRows, "sys_role_menu 原样双轨行必须存在")

	// (c) 置空 menuIds（全回收）→ 同一 token 再请求 403
	status, body = doAuthRequest(t, adminToken(), "PUT", ts.URL+"/api/v1/roles/"+roleIDStr+"/menus", `{"menuIds":[]}`)
	require.Equal(t, http.StatusOK, status, "全回收必须成功: body=%s", body)
	status, body = doAuthRequest(t, uToken, "GET", ts.URL+"/api/v1/users", "")
	require.Equal(t, http.StatusForbidden, status, "回收后同一 token 下一请求必须 403: body=%s", body)

	// (d2) 双轨同删：p 行与 sys_role_menu 行同步消失
	require.NoError(t, db.Raw(
		"SELECT (SELECT COUNT(*) FROM casbin_rule WHERE ptype='p' AND v0=? AND v1='system:user:list') AS p_rows, "+
			"(SELECT COUNT(*) FROM sys_role_menu WHERE role_id=? AND menu_id=?) AS rm_rows",
		roleCode, roleID, menuID,
	).Scan(&count).Error)
	assert.Equal(t, int64(0), count.PRows, "全回收后 casbin_rule p 行必须消失")
	assert.Equal(t, int64(0), count.RMRows, "全回收后 sys_role_menu 行必须同步消失（两轨同写同删）")
}

// TestPermRevokeRoleImmediate 角色回收即时生效（用户轴）：分配角色+菜单
// 得 200 后，admin PUT /users/{U}/roles []（清空=清 g 行）→ 同一 U token 下一
// 请求 403——g 行回收不重登录即时生效。
func TestPermRevokeRoleImmediate(t *testing.T) {
	ts, dsn, adminToken, _ := newTestApp(t)

	const roleCode = "perm_revoke_role"
	menuID, roleID, userID := permFixture(t, dsn, roleCode, "perm-revoke-user")
	roleIDStr := strconv.FormatInt(roleID, 10)
	userIDStr := strconv.FormatInt(userID, 10)
	menuIDStr := strconv.FormatInt(menuID, 10)

	// 前提链：分配角色 + 分配菜单 → U 登录 → 200
	status, body := doAuthRequest(t, adminToken(), "PUT", ts.URL+"/api/v1/users/"+userIDStr+"/roles",
		`{"roleIds":["`+roleIDStr+`"]}`)
	require.Equal(t, http.StatusOK, status, "分配角色必须成功: body=%s", body)
	status, body = doAuthRequest(t, adminToken(), "PUT", ts.URL+"/api/v1/roles/"+roleIDStr+"/menus",
		`{"menuIds":["`+menuIDStr+`"]}`)
	require.Equal(t, http.StatusOK, status, "分配菜单必须成功: body=%s", body)
	uToken := loginAs(t, ts, "perm-revoke-user", testAdminPass)
	status, body = doAuthRequest(t, uToken, "GET", ts.URL+"/api/v1/users", "")
	require.Equal(t, http.StatusOK, status, "回收前必须 200: body=%s", body)

	// 清空角色（g 行全回收）→ 同一 token 下一请求 403
	status, body = doAuthRequest(t, adminToken(), "PUT", ts.URL+"/api/v1/users/"+userIDStr+"/roles", `{"roleIds":[]}`)
	require.Equal(t, http.StatusOK, status, "角色回收必须成功: body=%s", body)
	status, body = doAuthRequest(t, uToken, "GET", ts.URL+"/api/v1/users", "")
	require.Equal(t, http.StatusForbidden, status, "角色回收后同一 token 下一请求必须 403（g 层即时生效）: body=%s", body)
}

// TestPermDeleteCascades 角色删除级联清策（预清模式回归——清理
// 前置于四连删事务，删除成功即策略已撤、无 post-commit 步骤）：分配菜单产生 p 行后
// 删除角色 R → casbin_rule 无 R 的任何 p 行（v0）与 g 行（v1）——ClearRole 双清。
func TestPermDeleteCascades(t *testing.T) {
	ts, dsn, adminToken, _ := newTestApp(t)
	db := openCheckDB(t, dsn)

	const roleCode = "perm_delete_cascade"
	menuID, roleID, _ := permFixture(t, dsn, roleCode, "perm-cascade-user")
	roleIDStr := strconv.FormatInt(roleID, 10)
	menuIDStr := strconv.FormatInt(menuID, 10)

	// 分配菜单产生 p 行（SysRolePerms 写入）
	status, body := doAuthRequest(t, adminToken(), "PUT", ts.URL+"/api/v1/roles/"+roleIDStr+"/menus",
		`{"menuIds":["`+menuIDStr+`"]}`)
	require.Equal(t, http.StatusOK, status, "分配菜单必须成功: body=%s", body)
	var before struct {
		Cnt int64
	}
	require.NoError(t, db.Raw(
		"SELECT COUNT(*) AS cnt FROM casbin_rule WHERE (ptype='p' AND v0=?) OR (ptype='g' AND v1=?)",
		roleCode, roleCode,
	).Scan(&before).Error)
	require.Equal(t, int64(1), before.Cnt, "删除前必须恰 1 行 p 策略")

	// 删除角色 → casbin_rule 无 R 的任何 p/g 行（ClearRole 双清）
	status, body = doAuthRequest(t, adminToken(), "DELETE", ts.URL+"/api/v1/roles/"+roleIDStr, "")
	require.Equal(t, http.StatusOK, status, "删除角色必须成功: body=%s", body)
	require.NoError(t, db.Raw(
		"SELECT COUNT(*) AS cnt FROM casbin_rule WHERE (ptype='p' AND v0=?) OR (ptype='g' AND v1=?)",
		roleCode, roleCode,
	).Scan(&before).Error)
	assert.Equal(t, int64(0), before.Cnt, "删除角色后 casbin_rule 必须无该角色任何 p/g 行（ClearRole 双清）")
}

// TestPermRouteLockdown 全量收口横向越权采样（清偿直证）：
// 角色 R + 用户 U 经 API 授权链装配后逐域采样六端点——
//
//	(a) R 无任何 p 策略：U 同一 token 每域采样一条（users/menus/depts.tree/roles/
//	    api-keys/kickout）全部 403/10200（漏挂即在此暴露——缓解断言）；
//	(b) admin 给 R 分配【仅含 system:menu:list】菜单（PUT /roles/:id/menus）→
//	    **同一 U token 不重登录**：GET /menus 200 且 GET /users 仍 403——授权
//	    最小化与横向隔离直证（同 token 授权域 200 与未授权域 403 并存）；
//	(c) kickout 叠加语义（纵深防御直证）：R 追加 system:auth:kickout 菜单
//	    （sys_menu 无 auth 域行，发明串直插菜单——Q2 已知边界）→ GET /menus 仍
//	    200（p 行同步链正常）→ U 调 kickout 仍 403：RequirePerm 层已放行，403
//	    必来自 RequireRoles(super_admin) 第二层拦截；
//	(d) super_admin 调 kickout → 200（matcher 豁免 + RequireRoles 命中；目标为
//	    无会话孤儿用户，KickoutUser 幂等 no-op 不扰动测试会话）。
func TestPermRouteLockdown(t *testing.T) {
	ts, dsn, adminToken, _ := newTestApp(t)
	db := openCheckDB(t, dsn)
	adminTk := adminToken()

	const roleCode = "lockdown_role"
	menuMenu := int64(ids.NewID())
	execMust(t, db,
		"INSERT INTO sys_menu (id, name, permission, create_time, update_time) VALUES (?, '菜单列表', 'system:menu:list', NOW(), NOW())",
		menuMenu,
	)
	menuKick := int64(ids.NewID())
	execMust(t, db,
		"INSERT INTO sys_menu (id, name, permission, create_time, update_time) VALUES (?, '踢下线', 'system:auth:kickout', NOW(), NOW())",
		menuKick,
	)
	roleID := seedRoleWithScope(t, dsn, "收口采样角色", roleCode, false, 5)
	userID := seedUser(t, dsn, "lockdown-user", testAdminHash, "采样者", 1, false)
	roleIDStr := strconv.FormatInt(roleID, 10)
	userIDStr := strconv.FormatInt(userID, 10)
	orphanID := seedUser(t, dsn, "lockdown-orphan", testAdminHash, "孤儿", 1, false)
	kickBody := `{"userId":"` + strconv.FormatInt(orphanID, 10) + `"}`

	assignR := func(menuIDsJSON string) {
		t.Helper()
		status, body := doAuthRequest(t, adminTk, "PUT", ts.URL+"/api/v1/roles/"+roleIDStr+"/menus",
			`{"menuIds":[`+menuIDsJSON+`]}`)
		require.Equal(t, http.StatusOK, status, "授菜单必须成功: body=%s", body)
	}

	// (a) U 挂 R（R 无 p 策略）→ 登录 → 每域采样一条全 403/10200
	status, body := doAuthRequest(t, adminTk, "PUT", ts.URL+"/api/v1/users/"+userIDStr+"/roles",
		`{"roleIds":["`+roleIDStr+`"]}`)
	require.Equal(t, http.StatusOK, status, "分配角色必须成功: body=%s", body)
	uToken := loginAs(t, ts, "lockdown-user", testAdminPass)
	for _, ep := range []struct{ method, path, reqBody string }{
		{"GET", "/api/v1/users", ""},
		{"GET", "/api/v1/menus", ""},
		{"GET", "/api/v1/depts/tree", ""},
		{"GET", "/api/v1/roles", ""},
		{"GET", "/api/v1/api-keys", ""},
		{"POST", "/api/v1/auth/kickout", kickBody},
	} {
		status, body := doAuthRequest(t, uToken, ep.method, ts.URL+ep.path, ep.reqBody)
		require.Equal(t, http.StatusForbidden, status, "%s %s 无 p 策略必须 403（漏挂/perm 串错在此暴露）: body=%s", ep.method, ep.path, body)
		var env struct {
			Code int `json:"code"`
		}
		require.NoError(t, json.Unmarshal(body, &env))
		assert.Equal(t, 10200, env.Code, "%s %s body=%s", ep.method, ep.path, body)
	}

	// (b) 仅授 system:menu:list → 同一 token：授权域 200 + 未授权域 403 并存
	menuMenuStr := strconv.FormatInt(menuMenu, 10)
	assignR(`"` + menuMenuStr + `"`)
	status, body = doAuthRequest(t, uToken, "GET", ts.URL+"/api/v1/menus", "")
	require.Equal(t, http.StatusOK, status, "授权域 menus 必须同 token 即时 200: body=%s", body)
	status, body = doAuthRequest(t, uToken, "GET", ts.URL+"/api/v1/users", "")
	require.Equal(t, http.StatusForbidden, status, "未授权域 users 同一 token 必须仍 403（横向隔离直证）: body=%s", body)
	var stillDenied struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(body, &stillDenied))
	assert.Equal(t, 10200, stillDenied.Code, "body=%s", body)

	// (c) R 追加 kickout 菜单（全量替换语义）→ p 行同步正常（menus 仍 200）→
	// U 调 kickout 仍 403——RequireRoles 第二层拦截直证（纵深防御）
	menuKickStr := strconv.FormatInt(menuKick, 10)
	assignR(`"` + menuMenuStr + `", "` + menuKickStr + `"`)
	status, body = doAuthRequest(t, uToken, "GET", ts.URL+"/api/v1/menus", "")
	require.Equal(t, http.StatusOK, status, "追加菜单后授权链路必须仍正常（p 行已含 kickout 串）: body=%s", body)
	// 直查 casbin_rule：U 的 g 行 + R 的 kickout p 行均已就位——403 非授权缺失
	var policyRow struct {
		Cnt int64
	}
	require.NoError(t, db.Raw(
		"SELECT COUNT(*) AS cnt FROM casbin_rule WHERE (ptype='p' AND v0=? AND v1='system:auth:kickout') OR (ptype='g' AND v1=?)",
		roleCode, roleCode,
	).Scan(&policyRow).Error)
	require.Equal(t, int64(2), policyRow.Cnt, "p(kickout)+g 行必须均已同步（403 必来自第二层）")
	status, body = doAuthRequest(t, uToken, "POST", ts.URL+"/api/v1/auth/kickout", kickBody)
	require.Equal(t, http.StatusForbidden, status, "持 kickout p 策略的非 super_admin 调 kickout 必须仍 403（RequireRoles 第二层）: body=%s", body)

	// (d) super_admin 调 kickout → 200（matcher 豁免过 RequirePerm + RequireRoles
	// 命中；目标孤儿用户无会话，KickoutUser 幂等 no-op）
	status, body = doAuthRequest(t, adminTk, "POST", ts.URL+"/api/v1/auth/kickout", kickBody)
	require.Equal(t, http.StatusOK, status, "super_admin kickout 必须 200（叠加双层全放行）: body=%s", body)
}

// TestUserDataScope dataScope 五档行差异（验收核心，四步断言
// 法第 4 步）。种子数据集：dept 三层树 D1(根)←D1a←D1a1 + 独立 D2；用户 C 挂 D1、
// A 挂 D1a1、B 挂 D2（create_by_id=admin），另有 create_by_id=C 的自建行落 D2
// （档 1 只认 create_by_id 不认 dept——落 D2 以与档 2/4 行集区分）。对 C 依次配
// 单一角色五档断言（每档角色 seed 后 AssignRoles，**同一 C token 不重登录**逐档
// 验证）：
//
//	档 1 → 恰 create_by_id=C 的行；档 2 → 恰 D1 用户（含 C 不含 A/B——跨部门
//	不可见验收明文）；档 3 → 恰 {D1,D1a,D1a1} 用户（含 A）；档 4 → PUT
//	/roles/:id/scopes [D2] 后恰 D2 用户（含 B）；档 5 → 全量。
//
// 补充断言：多角色取宽（档 2+档 5 并存 → 全量，max 聚合）；无有效档位 fail-closed
// （仅 NULL 档角色持 list perm → 本人档——纯无角色被 RequirePerm 403 挡在授权层，
// 本断言是其经授权面的可观察形态）；total 与 items 行数一致（同过滤闭包）；
// 档位变更即时（PUT /roles/:id 改 dataScope 2→5 后同一 token 下一请求即全量，
// 表驱动实时查询无缓存）。
func TestUserDataScope(t *testing.T) {
	ts, dsn, adminToken, adminID := newTestApp(t)
	db := openCheckDB(t, dsn)
	token := adminToken()

	// --- 种子：dept 三层树 + 独立部门 ---
	var d1, d1a, d1a1, d2 int64 = 990701, 990702, 990703, 990704
	seedDept(t, dsn, d1, "根部门")
	seedDeptChild(t, dsn, d1a, d1, "子部门")
	seedDeptChild(t, dsn, d1a1, d1a, "孙部门")
	seedDept(t, dsn, d2, "独立部门")

	// --- 种子：用户 C/A/B（creator=admin）+ C 的自建行（create_by_id=C，落 D2）---
	cID := seedUserWithDeptCreator(t, dsn, "ds-user-c", testAdminHash, "用户C", 1, false, &d1, &adminID)
	_ = seedUserWithDeptCreator(t, dsn, "ds-user-a", testAdminHash, "用户A", 1, false, &d1a1, &adminID)
	_ = seedUserWithDeptCreator(t, dsn, "ds-user-b", testAdminHash, "用户B", 1, false, &d2, &adminID)
	_ = seedUserWithDeptCreator(t, dsn, "ds-self-row", testAdminHash, "自建行", 1, false, &d2, &cID)

	// --- 种子：含 system:user:list perm 的菜单 M（各档角色的授权入口）---
	menuID := int64(ids.NewID())
	menuIDStr := strconv.FormatInt(menuID, 10)
	execMust(t, db,
		"INSERT INTO sys_menu (id, name, permission, create_time, update_time) VALUES (?, '用户列表', 'system:user:list', NOW(), NOW())",
		menuID,
	)

	// --- 种子：五档角色 + NULL 档角色（全部先挂 M 授 list perm）---
	role1 := seedRoleWithScope(t, dsn, "档一角色", "ds_mode1_role", false, 1)
	role2 := seedRoleWithScope(t, dsn, "档二角色", "ds_mode2_role", false, 2)
	role3 := seedRoleWithScope(t, dsn, "档三角色", "ds_mode3_role", false, 3)
	role4 := seedRoleWithScope(t, dsn, "档四角色", "ds_mode4_role", false, 4)
	role5 := seedRoleWithScope(t, dsn, "档五角色", "ds_mode5_role", false, 5)
	roleNull := seedRole(t, dsn, "空档角色", "ds_null_role", false)
	for _, rid := range []int64{role1, role2, role3, role4, role5, roleNull} {
		status, body := doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/roles/"+strconv.FormatInt(rid, 10)+"/menus",
			`{"menuIds":["`+menuIDStr+`"]}`)
		require.Equal(t, http.StatusOK, status, "授菜单必须成功: body=%s", body)
	}

	// --- helper：C 登录一次（五档逐档验证同一 token 不重登录——同轴）---
	cToken := loginAs(t, ts, "ds-user-c", testAdminPass)

	assignRoles := func(roleIDs ...int64) {
		t.Helper()
		ids := make([]string, 0, len(roleIDs))
		for _, r := range roleIDs {
			ids = append(ids, strconv.FormatInt(r, 10))
		}
		status, body := doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/users/"+strconv.FormatInt(cID, 10)+"/roles",
			`{"roleIds":["`+strings.Join(ids, `","`)+`"]}`)
		require.Equal(t, http.StatusOK, status, "分配角色必须成功: body=%s", body)
	}
	listAs := func() (map[string]bool, int64) {
		t.Helper()
		status, body := doAuthRequest(t, cToken, "GET", ts.URL+"/api/v1/users?page=1&size=50", "")
		require.Equal(t, http.StatusOK, status, "C 列表必须 200: body=%s", body)
		var env struct {
			Data struct {
				Items []struct {
					Username string `json:"username"`
				} `json:"items"`
				Total int64 `json:"total"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(body, &env), "body=%s", body)
		set := make(map[string]bool, len(env.Data.Items))
		for _, it := range env.Data.Items {
			set[it.Username] = true
		}
		assert.Equal(t, env.Data.Total, int64(len(env.Data.Items)), "total 与 items 行数必须一致（P15 同过滤闭包）: body=%s", body)
		return set, env.Data.Total
	}

	// --- 档 1：本人档 → 恰 create_by_id=C 的行 ---
	assignRoles(role1)
	set, total := listAs()
	assert.Equal(t, int64(1), total, "档 1 必须恰 1 行: set=%v", set)
	assert.Equal(t, map[string]bool{"ds-self-row": true}, set, "档 1 行集必须恰为 create_by_id=C 的行: %v", set)

	// --- 档 2：本部门 → 恰 D1 用户（含 C 不含 A/B——跨部门不可见验收明文）---
	assignRoles(role2)
	set, total = listAs()
	assert.Equal(t, int64(1), total, "档 2 必须恰 1 行: set=%v", set)
	assert.Equal(t, map[string]bool{"ds-user-c": true}, set, "档 2 行集必须恰为本部门 D1 用户（他部门 A/B 不可见）: %v", set)

	// --- 档 3：本部门及以下 → 恰 {D1, D1a, D1a1} 用户（含孙部门 A）---
	assignRoles(role3)
	set, total = listAs()
	assert.Equal(t, int64(2), total, "档 3 必须恰 2 行: set=%v", set)
	assert.Equal(t, map[string]bool{"ds-user-c": true, "ds-user-a": true}, set, "档 3 行集必须恰为 D1 子树用户: %v", set)

	// --- 档 4：先 PUT scopes [D2] 配部门集合 → 恰 D2 用户（含 B）---
	status, body := doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/roles/"+strconv.FormatInt(role4, 10)+"/scopes",
		`{"deptIds":["`+strconv.FormatInt(d2, 10)+`"]}`)
	require.Equal(t, http.StatusOK, status, "配置档 4 scopes 必须成功: body=%s", body)
	assignRoles(role4)
	set, total = listAs()
	assert.Equal(t, int64(2), total, "档 4 必须恰 2 行: set=%v", set)
	assert.Equal(t, map[string]bool{"ds-user-b": true, "ds-self-row": true}, set, "档 4 行集必须恰为 sys_role_dept 集合 D2 用户: %v", set)

	// --- 档 5：全部 → 全量（admin 种子 + 4 创建行 = 5 行）---
	assignRoles(role5)
	set, total = listAs()
	assert.Equal(t, int64(5), total, "档 5 必须全量 5 行: set=%v", set)
	assert.True(t, set["ds-user-a"] && set["ds-user-b"] && set["ds-user-c"] && set["ds-self-row"], "档 5 必须含全部测试用户: %v", set)

	// --- 多角色取宽：档 2 + 档 5 并存 → max=5 全量 ---
	assignRoles(role2, role5)
	set, total = listAs()
	assert.Equal(t, int64(5), total, "多角色必须取最宽档 max(dataScope)=5 → 全量: set=%v", set)

	// --- 无有效档位 fail-closed：仅 NULL 档角色（有 list perm）→ 本人档 ---
	assignRoles(roleNull)
	set, total = listAs()
	assert.Equal(t, int64(1), total, "无有效档位必须回退最窄档恰 1 行: set=%v", set)
	assert.Equal(t, map[string]bool{"ds-self-row": true}, set, "全部角色档位 NULL 必须按最窄档（本人）fail-closed: %v", set)

	// --- 档位变更即时（表驱动实时查询无缓存）：role2 dataScope 2→5，同一 token 下一请求全量 ---
	assignRoles(role2)
	set, total = listAs()
	assert.Equal(t, int64(1), total, "变更前必须仍为档 2 行集: set=%v", set)
	status, body = doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/roles/"+strconv.FormatInt(role2, 10), `{"dataScope":5}`)
	require.Equal(t, http.StatusOK, status, "改档位必须成功: body=%s", body)
	set, total = listAs()
	assert.Equal(t, int64(5), total, "档位变更后同一 token 下一请求必须按新档全量（无缓存直证）: set=%v", set)
}

// TestRoleDisableRevokeImmediate 角色停用对授权即刻生效（回归，停用家族
// 语义在角色轴的对应物）：分配角色+菜单 → U 同一 token 200 → PUT status=DISABLED →
// 同一 token 下一请求 403 且 casbin p/g 行双清（停用=授权即刻撤销）→ 恢复 ENABLED
// → 同一 token 200 且 p/g 行按业务表重建（恢复即刻复原，不等重启对账）。boot 对账
// 同口径直证：repo 直调 FindUserRoleCodes/FindRolePerms 不含停用角色行（重启导入
// 与运行期同口径，停用 super_admin 的豁免 g 行绝不可经对账还魂）。
func TestRoleDisableRevokeImmediate(t *testing.T) {
	ts, dsn, adminToken, _ := newTestApp(t)
	db := openCheckDB(t, dsn)

	const roleCode = "perm_disable_revoke"
	menuID, roleID, userID := permFixture(t, dsn, roleCode, "perm-disable-user")
	roleIDStr := strconv.FormatInt(roleID, 10)
	userIDStr := strconv.FormatInt(userID, 10)
	menuIDStr := strconv.FormatInt(menuID, 10)

	// 前提链：分配角色 + 分配菜单 → U 登录 → 200
	status, body := doAuthRequest(t, adminToken(), "PUT", ts.URL+"/api/v1/users/"+userIDStr+"/roles",
		`{"roleIds":["`+roleIDStr+`"]}`)
	require.Equal(t, http.StatusOK, status, "分配角色必须成功: body=%s", body)
	status, body = doAuthRequest(t, adminToken(), "PUT", ts.URL+"/api/v1/roles/"+roleIDStr+"/menus",
		`{"menuIds":["`+menuIDStr+`"]}`)
	require.Equal(t, http.StatusOK, status, "分配菜单必须成功: body=%s", body)
	uToken := loginAs(t, ts, "perm-disable-user", testAdminPass)
	status, body = doAuthRequest(t, uToken, "GET", ts.URL+"/api/v1/users", "")
	require.Equal(t, http.StatusOK, status, "停用前必须 200: body=%s", body)

	casbinRows := func() int64 {
		t.Helper()
		var n int64
		require.NoError(t, db.Raw(
			"SELECT COUNT(*) FROM casbin_rule WHERE (ptype='p' AND v0=?) OR (ptype='g' AND v1=?)",
			roleCode, roleCode,
		).Scan(&n).Error)
		return n
	}
	require.Equal(t, int64(2), casbinRows(), "停用前 p+g 行必须齐备（2 行）")

	// 停用角色 → 同一 token 下一请求 403（运行期即刻失效）+ casbin 双清
	status, body = doAuthRequest(t, adminToken(), "PUT", ts.URL+"/api/v1/roles/"+roleIDStr, `{"status":"DISABLED"}`)
	require.Equal(t, http.StatusOK, status, "停用角色必须成功: body=%s", body)
	status, body = doAuthRequest(t, uToken, "GET", ts.URL+"/api/v1/users", "")
	require.Equal(t, http.StatusForbidden, status, "停用角色后同一 token 下一请求必须 403（即刻生效）: body=%s", body)
	var forbidden struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(body, &forbidden))
	assert.Equal(t, 10200, forbidden.Code, "body=%s", body)
	assert.Equal(t, int64(0), casbinRows(), "停用后 casbin p/g 行必须双清（ClearRole）")

	// 恢复启用 → 同一 token 下一请求 200（按业务表重建）+ casbin 双行复原
	status, body = doAuthRequest(t, adminToken(), "PUT", ts.URL+"/api/v1/roles/"+roleIDStr, `{"status":"ENABLED"}`)
	require.Equal(t, http.StatusOK, status, "恢复角色必须成功: body=%s", body)
	status, body = doAuthRequest(t, uToken, "GET", ts.URL+"/api/v1/users", "")
	require.Equal(t, http.StatusOK, status, "恢复后同一 token 下一请求必须 200（策略重建即刻生效）: body=%s", body)
	assert.Equal(t, int64(2), casbinRows(), "恢复后 casbin p/g 行必须按业务表重建（2 行）")

	// boot 对账同口径直证：停用角色（独立 seed：user_role + role_menu 关联齐备）
	// 不得出现在对账数据源——重启导入不会复活停用角色的任何策略行
	const offCode = "perm_disabled_reconcile"
	offRoleID := seedRoleWithScope(t, dsn, "停用对账角色", offCode, false, 5)
	attachRoleToUser(t, dsn, userID, offRoleID, "停用对账角色")
	offMenuID := int64(ids.NewID())
	execMust(t, db,
		"INSERT INTO sys_menu (id, name, permission, create_time, update_time) VALUES (?, '停用对账菜单', 'system:role:list', NOW(), NOW())",
		offMenuID,
	)
	execMust(t, db, "INSERT INTO sys_role_menu (id, role_id, role_name, menu_id, menu_name) VALUES (?, ?, ?, ?, '停用对账菜单')",
		int64(ids.NewID()), offRoleID, "停用对账角色", offMenuID)
	execMust(t, db, "UPDATE sys_role SET status = 0 WHERE id = ?", offRoleID)

	repo := repository.NewRelationRepo(db)
	ctx := context.Background()
	userRules, err := repo.FindUserRoleCodes(ctx)
	require.NoError(t, err)
	for _, ur := range userRules {
		assert.NotEqual(t, offCode, ur.RoleCode, "boot 对账 g 行数据源不得含停用角色（同口径）")
	}
	rolePerms, err := repo.FindRolePerms(ctx)
	require.NoError(t, err)
	for _, rp := range rolePerms {
		assert.NotEqual(t, offCode, rp.RoleCode, "boot 对账 p 行数据源不得含停用角色（同口径）")
	}
	// 启用角色 perm_disable_revoke 的行必须仍在（过滤只打停用行，不误伤）
	foundEnabled := false
	for _, rp := range rolePerms {
		if rp.RoleCode == roleCode {
			foundEnabled = true
		}
	}
	assert.True(t, foundEnabled, "启用角色的 p 行必须正常导入（过滤不误伤）")
}
