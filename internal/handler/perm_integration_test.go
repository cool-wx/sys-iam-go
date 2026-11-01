//go:build integration

// Casbin enforcer 装配与 requirePerm 即时生效集成测试（真实 MySQL+Redis 双容器，
// 复用 newTestApp 基建）。覆盖：boot 对账 g 行导入 + matcher 豁免独立于 p 行、
// 无角色 403/10200（PERM-06）、无 token 401、分配/回收同 token 下一请求生效
// （PERM-02，Plan Task 2 扩展）、双轨落库（PERM-04）、级联清理。
package handler_test

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

	// ① admin（super_admin 豁免走 enforcer g 层，D-07）→ 200
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

// TestPermHotReload 即时生效四步链（PERM-02/PERM-04/PERM-05 验收核心）：
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
	require.Equal(t, http.StatusOK, status, "分配后同一 token 下一请求必须 200（PERM-02 即时生效直证）: body=%s", body)

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
	assert.Equal(t, int64(1), count.PRows, "casbin_rule 必须存在 (p, role_code, perm) 行（PERM-04）")
	assert.Equal(t, int64(1), count.RMRows, "sys_role_menu 原样双轨行必须存在（PERM-04）")

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

// TestPermRevokeRoleImmediate 角色回收即时生效（PERM-02 用户轴）：分配角色+菜单
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

// TestPermDeleteCascades 角色删除级联清策（T-3-01）：分配菜单产生 p 行后删除
// 角色 R → casbin_rule 无 R 的任何 p 行（v0）与 g 行（v1）——ClearRole 双清。
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
