//go:build integration

// 用户装配域扩展集成测试：roles 子资源全量替换、状态停用
// 踢会话（安全增强）、重置密码踢会话、builtin 删除 409 拦截、删除清理关联、
// 两查询装配防 N+1——真实 MySQL+Redis 双容器。
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

// grantUserPerm 为运行时用户授权单条 perm（全量收口适配）：DB 直插 perm 菜单
// + seedRoleWithScope 角色，再走 API 授权链（PUT /roles/:id/menus + PUT /users/:id/roles）
// 同步 enforcer——g/p 行运行时写入使被测用户当前会话下一请求即持 perm。boot 对账只
// 覆盖 newTestApp 种子三件套；本 helper 服务 app.New 之后经 API 创建的用户：
// 管理端点全部挂 gate，探活必须经 API 授权链（纪律：不拆 gate）。
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

// TestUserRolesAssignReplace 角色分配全量替换语义：分配两角色 →
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

// TestUserRolesBuiltinAssignGuard builtin 角色分配提权守卫（回归）：
// 仅持 system:user:edit 的操作者把 builtin 角色分配给任意用户 → 403/10603 且
// sys_user_role 与 casbin g 行零变化（前置拦截零写调用）；super_admin 分配
// builtin 角色 → 200（守卫不误伤特权路径）。
func TestUserRolesBuiltinAssignGuard(t *testing.T) {
	ts, dsn, adminToken, _ := newTestApp(t)
	token := adminToken()
	seedDept(t, dsn, 100, "总部")

	uidStr := createRawUser(t, ts, token, `{"username":"guard-target","password":"pw-123456","realName":"守卫目标","deptId":"100"}`)
	uid, _ := strconv.ParseInt(uidStr, 10, 64)

	// 操作者：仅持 user:edit 的角色（无 super_admin）——攻击链主体
	editorID := seedUser(t, dsn, "guard-editor", testAdminHash, "编辑者", 1, false)
	grantUserPerm(t, ts, token, dsn, editorID, "system:user:edit", "guard_editor_role")
	editorToken := loginAs(t, ts, "guard-editor", testAdminPass)

	// super_admin 角色行（builtin=true，seed 直插——不可经 API 创建）
	superRoleID := seedRoleWithScope(t, dsn, "超级管理员", "guard_super_admin", true, 5)
	superRoleIDStr := strconv.FormatInt(superRoleID, 10)

	// 攻击尝试：editor 把 builtin super_admin 分配给目标用户 → 403/10603
	status, body := assignRoles(t, ts, editorToken, uidStr, `"`+superRoleIDStr+`"`)
	require.Equal(t, http.StatusForbidden, status, "非 super_admin 分配 builtin 角色必须 403: body=%s", body)
	var forbidden struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(body, &forbidden))
	assert.Equal(t, 10603, forbidden.Code, "业务码必须 10603: body=%s", body)
	assert.Contains(t, forbidden.Message, "系统内置角色仅超级管理员可分配", "body=%s", body)

	// 前置拦截零写调用：业务表与 casbin g 行均零变化
	checkDB := openCheckDB(t, dsn)
	require.Equal(t, int64(0), userRoleCount(t, checkDB, uid), "守卫必须前置拦截，sys_user_role 不得有写入")
	var gRows int64
	require.NoError(t, checkDB.Raw(
		"SELECT COUNT(*) FROM casbin_rule WHERE ptype = 'g' AND v1 = 'guard_super_admin'",
	).Scan(&gRows).Error)
	assert.Equal(t, int64(0), gRows, "casbin g 行必须零变化（提权链整体封死）")

	// 对比组：editor 分配普通角色 → 200（守卫不影响普通分配）
	normalRole := createRole(t, ts, token, "普通角色", "guard_normal_role")
	status, body = assignRoles(t, ts, editorToken, uidStr, `"`+normalRole+`"`)
	require.Equal(t, http.StatusOK, status, "非 builtin 角色分配必须不受守卫影响: body=%s", body)

	// 特权路径：super_admin 分配 builtin 角色 → 200（豁免者管理自有角色合法）
	status, body = assignRoles(t, ts, token, uidStr, `"`+superRoleIDStr+`"`)
	require.Equal(t, http.StatusOK, status, "super_admin 分配 builtin 角色必须放行: body=%s", body)
	require.NoError(t, checkDB.Raw(
		"SELECT COUNT(*) FROM casbin_rule WHERE ptype = 'g' AND v0 = ? AND v1 = 'guard_super_admin'",
		uidStr,
	).Scan(&gRows).Error)
	assert.Equal(t, int64(1), gRows, "特权分配后 g 行必须同步写入")
}

// TestUserRolesBuiltinAssignGuardRevokedSession 回归：守卫判据源是 enforcer
// g 层实时状态而非会话 roles 快照。REVIEW 攻击链逐步直证——受害者持 super_admin +
// editor（editor 带 system:user:edit p 行）登录（会话快照装配含 super_admin）→
// 管理员仅回收 super_admin（SyncUserRoles 删 g 行，会话不踢、快照不刷新）→ 同一
// 旧 token 经 RequirePerm（editor p 行放行）重分配 builtin → 403/10603。快照判据
// 此处放行 200（回收被旧会话完全抵消），g 层判据前置拦截。
func TestUserRolesBuiltinAssignGuardRevokedSession(t *testing.T) {
	ts, dsn, adminToken, _ := newTestApp(t)
	token := adminToken()
	seedDept(t, dsn, 100, "总部")
	checkDB := openCheckDB(t, dsn)

	// 受害者 + editor 角色（带 system:user:edit p 行）——回收后仍须过 RequirePerm
	// 门（editor g 行+p 行放行），攻击请求才能到达 AssignRoles 守卫
	victimID := seedUser(t, dsn, "revoke-victim", testAdminHash, "受害者", 1, false)
	victimIDStr := strconv.FormatInt(victimID, 10)
	editorRoleID := seedRoleWithScope(t, dsn, "编辑者", "revoke_editor_role", false, 5)
	editorRoleStr := strconv.FormatInt(editorRoleID, 10)
	execMust(t, checkDB,
		"INSERT INTO sys_menu (id, name, permission, create_time, update_time) VALUES (990491, '回收链菜单', 'system:user:edit', NOW(), NOW())")
	status, body := doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/roles/"+editorRoleStr+"/menus",
		`{"menuIds":["990491"]}`)
	require.Equal(t, http.StatusOK, status, "授菜单必须成功: body=%s", body)
	status, body = assignRoles(t, ts, token, victimIDStr, `"`+editorRoleStr+`"`)
	require.Equal(t, http.StatusOK, status, "授编辑者角色必须成功: body=%s", body)

	// newTestApp 已 seed role_code='super_admin' 的 builtin 角色（uk_role_code 唯一，
	// 不可再插同行）——查库取 id（守卫判据按 role_code 字面量 'super_admin' 匹配，
	// g 行角色码必须精确等于该串）
	var superRoleID int64
	require.NoError(t, checkDB.Raw("SELECT id FROM sys_role WHERE role_code = 'super_admin'").Scan(&superRoleID).Error)
	require.NotZero(t, superRoleID, "种子 super_admin 角色必须存在")
	superRoleIDStr := strconv.FormatInt(superRoleID, 10)

	// 管理员把 super_admin + editor 分配给受害者 → g 行双写（回收前基线）
	status, body = assignRoles(t, ts, token, victimIDStr, `"`+editorRoleStr+`", "`+superRoleIDStr+`"`)
	require.Equal(t, http.StatusOK, status, "管理员分配 super_admin+editor 必须 200: body=%s", body)

	// 受害者登录：会话快照装配含 super_admin（此刻 g 层与快照一致）
	victimToken := loginAs(t, ts, "revoke-victim", testAdminPass)

	// 回收前对照：受害者（g 层实时持 super_admin）重分配 builtin → 200
	// （守卫不误伤真实豁免者；全量替换含双角色保住 editor g 行）
	status, body = assignRoles(t, ts, victimToken, victimIDStr, `"`+superRoleIDStr+`", "`+editorRoleStr+`"`)
	require.Equal(t, http.StatusOK, status, "回收前 g 层持 super_admin 者分配 builtin 必须 200: body=%s", body)

	// 管理员回收：全量替换仅剩 editor → SyncUserRoles 删 (victim, super_admin) g 行
	// （AssignRoles 不踢会话、不刷新快照——攻击链的窗口前提）
	status, body = assignRoles(t, ts, token, victimIDStr, `"`+editorRoleStr+`"`)
	require.Equal(t, http.StatusOK, status, "管理员回收 super_admin 必须 200: body=%s", body)
	var gCnt int64
	require.NoError(t, checkDB.Raw(
		"SELECT COUNT(*) FROM casbin_rule WHERE ptype = 'g' AND v0 = ? AND v1 = 'super_admin'",
		victimIDStr,
	).Scan(&gCnt).Error)
	require.Equal(t, int64(0), gCnt, "前置条件：回收后 super_admin g 行必须已删除")

	// 核心断言：同一旧 token（快照仍含 super_admin、RequirePerm 经 editor
	// p 行放行）立即重分配 builtin → 403/10603
	status, body = assignRoles(t, ts, victimToken, victimIDStr, `"`+superRoleIDStr+`", "`+editorRoleStr+`"`)
	require.Equal(t, http.StatusForbidden, status, "回收后旧会话重分配 builtin 必须 403: body=%s", body)
	var forbidden struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(body, &forbidden))
	assert.Equal(t, 10603, forbidden.Code, "业务码必须 10603: body=%s", body)
	assert.Contains(t, forbidden.Message, "系统内置角色仅超级管理员可分配", "body=%s", body)

	// 双轨零写证明：守卫前置拦截，super_admin g 行保持删除、sys_user_role 保持仅
	// editor（回收不被旧会话抵消）
	require.NoError(t, checkDB.Raw(
		"SELECT COUNT(*) FROM casbin_rule WHERE ptype = 'g' AND v0 = ? AND v1 = 'super_admin'",
		victimIDStr,
	).Scan(&gCnt).Error)
	assert.Equal(t, int64(0), gCnt, "被拒请求不得复活 super_admin g 行（前置拦截零写）")
	var urCnt int64
	require.NoError(t, checkDB.Raw(
		"SELECT COUNT(*) FROM sys_user_role ur JOIN sys_role r ON ur.role_id = r.id WHERE ur.user_id = ? AND r.role_code = 'super_admin'",
		victimID,
	).Scan(&urCnt).Error)
	assert.Equal(t, int64(0), urCnt, "被拒请求不得写入 sys_user_role 关联（前置拦截零写）")
}

// TestAssignRolesDisabledRoleRejected 停用角色写路径封口（回归，用户轴）：
// 请求集合含停用角色时 AssignRoles 必须 400 前置拒绝——否则 SyncUserRoles 会把 g
// 行写回，停用语义被合法 API 复活（matcher 只看 casbin 行、无 status 概念，g 行即
// 权限）。断言单停用角色与混合集合（正常+停用）均整体拒绝、casbin g 行与
// sys_user_role 零写入；恢复 ENABLED 后分配照常（守卫不误伤正常路径）。
func TestAssignRolesDisabledRoleRejected(t *testing.T) {
	ts, dsn, adminToken, _ := newTestApp(t)
	token := adminToken()
	seedDept(t, dsn, 100, "总部")
	checkDB := openCheckDB(t, dsn)

	uidStr := createRawUser(t, ts, token, `{"username":"assign-disabled","password":"pw-123456","realName":"停用分配","deptId":"100"}`)
	uid, _ := strconv.ParseInt(uidStr, 10, 64)
	roleA := createRole(t, ts, token, "正常角色", "wronly_role_a")
	disabledRole := createRole(t, ts, token, "将停用角色", "wronly_role_b")

	// 停用 disabledRole（基线：尚未有任何关联，停用后 ClearRole 无行可清）
	status, body := doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/roles/"+disabledRole, `{"status":"DISABLED"}`)
	require.Equal(t, http.StatusOK, status, "停用必须成功: body=%s", body)

	// 核心：单停用角色分配 → 400
	status, body = assignRoles(t, ts, token, uidStr, `"`+disabledRole+`"`)
	require.Equal(t, http.StatusBadRequest, status, "含停用角色的分配必须 400: body=%s", body)
	var rejEnv struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(body, &rejEnv))
	assert.Equal(t, 10400, rejEnv.Code, "body=%s", body)
	assert.Contains(t, rejEnv.Message, "角色已停用", "body=%s", body)
	assert.Contains(t, rejEnv.Message, disabledRole, "message 应含停用角色 id: body=%s", body)

	// 混合集合（正常+停用）同样整体拒绝（任一停用即拒，不含糊放行）
	status, body = assignRoles(t, ts, token, uidStr, `"`+roleA+`", "`+disabledRole+`"`)
	require.Equal(t, http.StatusBadRequest, status, "混合集合必须整体拒绝: body=%s", body)

	// 零写证明：sys_user_role 与 casbin g 行零写入
	require.Equal(t, int64(0), userRoleCount(t, checkDB, uid), "停用角色分配必须前置拦截，sys_user_role 零写入")
	var gCnt int64
	require.NoError(t, checkDB.Raw(
		"SELECT COUNT(*) FROM casbin_rule WHERE ptype = 'g' AND v0 = ?", uidStr,
	).Scan(&gCnt).Error)
	assert.Equal(t, int64(0), gCnt, "停用角色分配必须前置拦截，casbin g 行零写入")

	// 恢复后分配照常（守卫不误伤正常路径）
	status, body = doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/roles/"+disabledRole, `{"status":"ENABLED"}`)
	require.Equal(t, http.StatusOK, status, "恢复必须成功: body=%s", body)
	status, body = assignRoles(t, ts, token, uidStr, `"`+roleA+`", "`+disabledRole+`"`)
	require.Equal(t, http.StatusOK, status, "恢复后含该角色的分配必须放行: body=%s", body)
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

// TestUserStatusDisableKickout 停用即踢存量会话三段直证：
// ①停用后旧 token 访问 401 ②停用用户登录被拒 10502 ③重新启用后可登录。
func TestUserStatusDisableKickout(t *testing.T) {
	ts, dsn, adminToken, _ := newTestApp(t)
	token := adminToken()
	seedDept(t, dsn, 100, "总部")

	uidStr := createRawUser(t, ts, token, `{"username":"status-user","password":"pw-123456","realName":"停用者","deptId":"100"}`)
	uid, err := strconv.ParseInt(uidStr, 10, 64)
	require.NoError(t, err)

	// 详情端点同样挂 requirePerm gate——探活用户必须持
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

// TestResetPasswordFlow 重置密码流：登录 tokenA → admin 重置
// （无旧密码校验）→ tokenA 即刻 401 → 用 123456 登录成功且 firstLogin=true → 旧密码
// 已被覆写失效。
func TestResetPasswordFlow(t *testing.T) {
	ts, dsn, adminToken, _ := newTestApp(t)
	token := adminToken()
	seedDept(t, dsn, 100, "总部")

	uidStr := createRawUser(t, ts, token, `{"username":"reset-user","password":"pw-123456","realName":"重置者","deptId":"100"}`)
	uid, err := strconv.ParseInt(uidStr, 10, 64)
	require.NoError(t, err)

	// 探活用户必须持 system:user:query（API 授权链，同上）
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

// TestDeleteBuiltinUser409 builtin 用户删除硬拦截：DELETE builtin=true
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
// sys_user_role 该 user_id 行数 0、casbin g 行（用户→角色）同步清零（预清
// 模式回归——预清在事务前执行，删除成功即策略已撤）、GET 404 域消息（先删关联
// 再删用户，源顺序）。
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
	// 双轨前置：casbin g 行已随 AssignRoles 同步写入
	var gRows int64
	require.NoError(t, checkDB.Raw(
		"SELECT COUNT(*) FROM casbin_rule WHERE ptype = 'g' AND v0 = ? AND v1 = 'role_del'",
		uidStr,
	).Scan(&gRows).Error)
	require.Equal(t, int64(1), gRows, "删除前 casbin g 行必须存在（双轨）")

	status, body = doAuthRequest(t, token, "DELETE", ts.URL+"/api/v1/users/"+uidStr, "")
	require.Equal(t, http.StatusOK, status, "普通用户删除应 200: body=%s", body)

	// 关联零残留 + 用户行已删 + casbin g 行同步清零（预清模式）
	require.Equal(t, int64(0), userRoleCount(t, checkDB, uid), "sys_user_role 关联必须清零")
	require.NoError(t, checkDB.Raw(
		"SELECT COUNT(*) FROM casbin_rule WHERE ptype = 'g' AND v0 = ? AND v1 = 'role_del'",
		uidStr,
	).Scan(&gRows).Error)
	assert.Equal(t, int64(0), gRows, "删除后 casbin g 行必须清零（预清模式，g 行不得残留至 TTL/重启）")
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

// TestUserListDetailRolesAssembly 两查询装配：3 用户各配不同角色 →
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

// TestDeleteUserKicksSessions 删除即踢存量会话（修复回归）：
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

	// 探活用户必须持 system:user:query（API 授权链，同上——
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

// TestUserStatusInvalidEnumRejected 状态变更端点非法枚举值 400（回归）：
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

// TestUserUpdateDeptNameNotWritable dept_name 不可经请求体直写（回归，
// "冗余不可伪造"truth 全覆盖）：Update 只带 deptName 不带 deptId →
// 200 但 dept_name 列保持服务端查库值不变（客户端展示冗余零直写路径）；带 deptId
// 变更时 dept_name 才以查库值联动覆写。
func TestUserUpdateDeptNameNotWritable(t *testing.T) {
	ts, dsn, adminToken, _ := newTestApp(t)
	token := adminToken()
	seedDept(t, dsn, 100, "总部")
	seedDept(t, dsn, 101, "研发部")

	uidStr := createRawUser(t, ts, token, `{"username":"deptname-guard","password":"pw-123456","realName":"冗余守卫","deptId":"100"}`)

	// 只带伪造 deptName（无 deptId）→ 200 但列值不动
	status, body := doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/users/"+uidStr,
		`{"deptName":"伪造部门名"}`)
	require.Equal(t, http.StatusOK, status, "update 请求本身应成功（deptName 键被忽略）: body=%s", body)
	checkDB := openCheckDB(t, dsn)
	var deptName string
	require.NoError(t, checkDB.Raw(
		"SELECT dept_name FROM sys_user WHERE username = 'deptname-guard'",
	).Scan(&deptName).Error)
	assert.Equal(t, "总部", deptName, "dept_name 必须保持服务端查库值，不得被请求体直写")

	// 带 deptId 变更 → dept_name 以查库值联动覆写（唯一合法路径）
	status, body = doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/users/"+uidStr,
		`{"deptId":"101","deptName":"又一次伪造"}`)
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	require.NoError(t, checkDB.Raw(
		"SELECT dept_name FROM sys_user WHERE username = 'deptname-guard'",
	).Scan(&deptName).Error)
	assert.Equal(t, "研发部", deptName, "deptId 变更时 dept_name 必须以查库值联动覆写")
}
