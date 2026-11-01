//go:build integration

// Casbin enforcer 装配与 requirePerm 即时生效集成测试（真实 MySQL+Redis 双容器，
// 复用 newTestApp 基建）。覆盖：boot 对账 g 行导入 + matcher 豁免独立于 p 行、
// 无角色 403/10200（PERM-06）、无 token 401、分配/回收同 token 下一请求生效
// （PERM-02，Plan Task 2 扩展）、双轨落库（PERM-04）、级联清理。
package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
