//go:build integration

package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/cool-wx/sys-iam-go/internal/event"
)

// TestDeptNameSync 部门改名端到端同步直证（Phase 5 SYNC-01/02 tracer，05-01 Task 2；
// 05-02 扩展同一文件覆盖其余 4 域）。链路：PUT 改名 → service 提交后发布
// DeptNameChanged → dispatcher 同步分发 → handler 独立事务幂等 UPDATE →
// sys_user.dept_name（dept_id 锚）与 sys_dept.parent_name（parent_id 锚）可断言。
// 覆盖五语义：Create 自动冗余基线、改名双表同步正例、参数化特殊字符（T-5-01）、
// 仅改备注零同步（发布条件负例）、幂等重放自愈 + 同名重放幂等拉齐（CR-01 修订：
// 显式带名即发布，同名重发经 handler `<>` 条件自愈，非零 UPDATE 短路）。
func TestDeptNameSync(t *testing.T) {
	ts, dsn, adminToken, _ := newTestApp(t)
	checkDB := openCheckDB(t, dsn)

	// --- seed（走 API 造真实链路：Create 路径的服务端冗余一并直证）---
	createDept := func(body string) int64 {
		status, respBody := doAuthRequest(t, adminToken(), "POST", ts.URL+"/api/v1/depts", body)
		require.Equal(t, http.StatusOK, status, "建部门失败: %s", respBody)
		var envelope struct {
			Code int `json:"code"`
			Data struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(respBody, &envelope), "body=%s", respBody)
		require.Equal(t, 0, envelope.Code, "body=%s", respBody)
		id, err := strconv.ParseInt(envelope.Data.ID, 10, 64)
		require.NoError(t, err, "data.id 必须是十进制字符串: %q", envelope.Data.ID)
		return id
	}
	putDept := func(id int64, body string) (int, string) {
		status, respBody := doAuthRequest(t, adminToken(), "PUT", ts.URL+"/api/v1/depts/"+strconv.FormatInt(id, 10), body)
		return status, string(respBody)
	}

	deptA := createDept(`{"deptName":"研发一部"}`)
	deptB := createDept(`{"deptName":"研发二部","parentId":"` + strconv.FormatInt(deptA, 10) + `"}`)
	userIDStr := createRawUser(t, ts, adminToken(), `{"username":"dept-sync-user","password":"pw-123456","realName":"同步用户","deptId":"`+strconv.FormatInt(deptA, 10)+`"}`)
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	require.NoError(t, err, "user id 必须是十进制字符串: %q", userIDStr)

	// Create 自动冗余基线直证：B.parent_name / U.dept_name 均为服务端查库冗余名
	requireSynced(t, userDeptNameOf(t, checkDB, userID), "研发一部", "Create 后 sys_user.dept_name 基线")
	requireSynced(t, deptParentNameOf(t, checkDB, deptB), "研发一部", "Create 后 sys_dept.parent_name 基线")

	// --- 正例：改名 A → A'，两关联表同步为新名 ---
	status, body := putDept(deptA, `{"deptName":"研发一部新名"}`)
	require.Equal(t, http.StatusOK, status, "改名失败: %s", body)
	requireSynced(t, userDeptNameOf(t, checkDB, userID), "研发一部新名", "改名后 sys_user.dept_name 同步（dept 事件表 1）")
	requireSynced(t, deptParentNameOf(t, checkDB, deptB), "研发一部新名", "改名后 sys_dept.parent_name 同步（dept 事件表 2）")

	// --- 参数化直证（T-5-01 缓解）：新名含单引号+双引号，占位符不破 SQL ---
	status, body = putDept(deptA, `{"deptName":"研'发\"引号部"}`)
	require.Equal(t, http.StatusOK, status, "特殊字符改名失败: %s", body)
	requireSynced(t, userDeptNameOf(t, checkDB, userID), "研'发\"引号部", "含引号新名 sys_user.dept_name 同步正确")
	requireSynced(t, deptParentNameOf(t, checkDB, deptB), "研'发\"引号部", "含引号新名 sys_dept.parent_name 同步正确")

	// --- 发布条件负例：仅改 remark 不发布 → 陈旧行保持陈旧（防无条件发布）---
	execMust(t, checkDB, "UPDATE sys_user SET dept_name = ? WHERE id = ?", "陈旧冗余名", userID)
	status, body = putDept(deptA, `{"remark":"仅改备注"}`)
	require.Equal(t, http.StatusOK, status, "仅改备注失败: %s", body)
	requireSynced(t, userDeptNameOf(t, checkDB, userID), "陈旧冗余名",
		"仅改 remark 不发布事件——sys_user.dept_name 必须保持陈旧值（未发布=未同步）")

	// --- 幂等重放：再次有效改名 → `<>` 条件把陈旧行拉齐（自愈直证）---
	status, body = putDept(deptA, `{"deptName":"研发三部"}`)
	require.Equal(t, http.StatusOK, status, "重放改名失败: %s", body)
	requireSynced(t, userDeptNameOf(t, checkDB, userID), "研发三部", "重放改名后陈旧 sys_user.dept_name 被 <> 拉齐（自愈）")
	requireSynced(t, deptParentNameOf(t, checkDB, deptB), "研发三部", "重放改名后 sys_dept.parent_name 同步")

	// --- 同名重放（CR-01 修订语义）：显式带名即发布——同名 PUT 仍发布 1 条事件，
	// handler 幂等 UPDATE（`冗余列 <> newName`）把陈旧哨兵值拉齐为新名（自愈直证；
	// 旧守卫「新名==旧行名不发布」下此请求零发布、哨兵值保持——该语义经 CR-01
	// 裁决废弃，与 service 层 name_sync_service_integration_test.go 形态 (c) 同款对齐）---
	execMust(t, checkDB, "UPDATE sys_user SET dept_name = ? WHERE id = ?", "哨兵值", userID)
	status, body = putDept(deptA, `{"deptName":"研发三部"}`)
	require.Equal(t, http.StatusOK, status, "同名重放失败: %s", body)
	requireSynced(t, userDeptNameOf(t, checkDB, userID), "研发三部",
		"同名 PUT 重发事件——sys_user.dept_name 哨兵值被幂等 UPDATE 拉齐（CR-01 自愈）")
}

// userDeptNameOf 查 sys_user.dept_name（DB 行级断言原料）。
func userDeptNameOf(t *testing.T, db *gorm.DB, userID int64) *string {
	t.Helper()
	var row struct {
		DeptName *string
	}
	require.NoError(t, db.Raw("SELECT dept_name FROM sys_user WHERE id = ?", userID).Scan(&row).Error)
	return row.DeptName
}

// deptParentNameOf 查 sys_dept.parent_name（DB 行级断言原料）。
func deptParentNameOf(t *testing.T, db *gorm.DB, deptID int64) *string {
	t.Helper()
	var row struct {
		ParentName *string
	}
	require.NoError(t, db.Raw("SELECT parent_name FROM sys_dept WHERE id = ?", deptID).Scan(&row).Error)
	return row.ParentName
}

// requireSynced 冗余列断言统一出口：非 nil 且值精确相等。
func requireSynced(t *testing.T, got *string, want, msg string) {
	t.Helper()
	require.NotNil(t, got, "%s（列值为 NULL）", msg)
	require.Equal(t, want, *got, msg)
}

// createViaAPI 经 API 建实体并解析 data.id（name_sync 三域测试共用的 seed 原料）。
func createViaAPI(t *testing.T, ts *httptest.Server, token, method, path, body string) int64 {
	t.Helper()
	status, respBody := doAuthRequest(t, token, method, ts.URL+path, body)
	require.Equal(t, http.StatusOK, status, "%s %s 失败: %s", method, path, respBody)
	var envelope struct {
		Code int `json:"code"`
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(respBody, &envelope), "body=%s", respBody)
	require.Equal(t, 0, envelope.Code, "body=%s", respBody)
	id, err := strconv.ParseInt(envelope.Data.ID, 10, 64)
	require.NoError(t, err, "data.id 必须是十进制字符串: %q", envelope.Data.ID)
	return id
}

// putViaAPI 经 API 提交更新（非创建路径——响应无 data.id 可解析，仅断言 200）。
func putViaAPI(t *testing.T, ts *httptest.Server, token, path, body string) {
	t.Helper()
	status, respBody := doAuthRequest(t, token, "PUT", ts.URL+path, body)
	require.Equal(t, http.StatusOK, status, "PUT %s 失败: %s", path, respBody)
}

// stringColOf 单行单列查询出口（冗余列断言原料——SELECT 必须恰 1 行）。
func stringColOf(t *testing.T, db *gorm.DB, query string, args ...any) *string {
	t.Helper()
	var row struct {
		Col *string
	}
	require.NoError(t, db.Raw(query, args...).Scan(&row).Error)
	return row.Col
}

// TestRoleNameSync 角色改名端到端同步直证（SYNC-01 role 事件 3 表）：sys_role_menu
// （AssignMenus 冗余写入）+ sys_user_role（AssignRoles 冗余写入）+ sys_role_dept
// （AssignScopes 冗余写入，Go 扩展 Q2）的 role_name 在 PUT 改名后全部同步为新名。
func TestRoleNameSync(t *testing.T) {
	ts, dsn, adminToken, _ := newTestApp(t)
	checkDB := openCheckDB(t, dsn)
	token := adminToken()

	roleID := createViaAPI(t, ts, token, "POST", "/api/v1/roles", `{"roleName":"巡检员","roleCode":"inspector"}`)
	menuID := createViaAPI(t, ts, token, "POST", "/api/v1/menus", `{"name":"巡检菜单","type":"MENU","path":"/inspect"}`)
	deptID := createViaAPI(t, ts, token, "POST", "/api/v1/depts", `{"deptName":"巡检部"}`)
	userIDStr := createRawUser(t, ts, token, `{"username":"role-sync-user","password":"pw-123456","realName":"角色同步用户","deptId":"`+strconv.FormatInt(deptID, 10)+`"}`)

	// 三张关联表 seed（走真实 API 链路——冗余列写入路径一并直证）：
	// sys_role_menu（AssignMenus）、sys_user_role（AssignRoles）、sys_role_dept（AssignScopes）。
	putViaAPI(t, ts, token, "/api/v1/roles/"+strconv.FormatInt(roleID, 10)+"/menus", `{"menuIds":["`+strconv.FormatInt(menuID, 10)+`"]}`)
	putViaAPI(t, ts, token, "/api/v1/users/"+userIDStr+"/roles", `{"roleIds":["`+strconv.FormatInt(roleID, 10)+`"]}`)
	putViaAPI(t, ts, token, "/api/v1/roles/"+strconv.FormatInt(roleID, 10)+"/scopes", `{"deptIds":["`+strconv.FormatInt(deptID, 10)+`"]}`)

	// 基线：三表 role_name 均为创建时冗余的旧名
	requireSynced(t, stringColOf(t, checkDB, "SELECT role_name AS col FROM sys_role_menu WHERE role_id = ?", roleID), "巡检员", "改名前 sys_role_menu.role_name 基线")
	requireSynced(t, stringColOf(t, checkDB, "SELECT role_name AS col FROM sys_user_role WHERE role_id = ?", roleID), "巡检员", "改名前 sys_user_role.role_name 基线")
	requireSynced(t, stringColOf(t, checkDB, "SELECT role_name AS col FROM sys_role_dept WHERE role_id = ?", roleID), "巡检员", "改名前 sys_role_dept.role_name 基线")

	// 改名 → 三表同步为新名（role 事件表 1/2/3，第三表 Go 扩展 Q2）
	putViaAPI(t, ts, token, "/api/v1/roles/"+strconv.FormatInt(roleID, 10), `{"roleName":"巡检长"}`)
	requireSynced(t, stringColOf(t, checkDB, "SELECT role_name AS col FROM sys_role_menu WHERE role_id = ?", roleID), "巡检长", "改名后 sys_role_menu.role_name 同步（role 事件表 1）")
	requireSynced(t, stringColOf(t, checkDB, "SELECT role_name AS col FROM sys_user_role WHERE role_id = ?", roleID), "巡检长", "改名后 sys_user_role.role_name 同步（role 事件表 2）")
	requireSynced(t, stringColOf(t, checkDB, "SELECT role_name AS col FROM sys_role_dept WHERE role_id = ?", roleID), "巡检长", "改名后 sys_role_dept.role_name 同步（role 事件表 3，Go 扩展 Q2）")
}

// TestMenuNameSync 菜单改名端到端同步直证（SYNC-01 menu 事件 2 表）：sys_role_menu
// .menu_name（menu_id 锚）+ sys_menu.parent_name（parent_id 锚，子菜单冗余）。
func TestMenuNameSync(t *testing.T) {
	ts, dsn, adminToken, _ := newTestApp(t)
	checkDB := openCheckDB(t, dsn)
	token := adminToken()

	menuID := createViaAPI(t, ts, token, "POST", "/api/v1/menus", `{"name":"菜单甲","type":"DIR","sort":1,"path":"/menu-a"}`)
	childID := createViaAPI(t, ts, token, "POST", "/api/v1/menus", `{"name":"子菜单","type":"MENU","parentId":"`+strconv.FormatInt(menuID, 10)+`","path":"/menu-a/child"}`)
	roleID := createViaAPI(t, ts, token, "POST", "/api/v1/roles", `{"roleName":"菜单观察员","roleCode":"menu_observer"}`)
	putViaAPI(t, ts, token, "/api/v1/roles/"+strconv.FormatInt(roleID, 10)+"/menus", `{"menuIds":["`+strconv.FormatInt(menuID, 10)+`"]}`)

	// 基线：role_menu.menu_name 与子菜单 parent_name 均为旧名
	requireSynced(t, stringColOf(t, checkDB, "SELECT menu_name AS col FROM sys_role_menu WHERE menu_id = ?", menuID), "菜单甲", "改名前 sys_role_menu.menu_name 基线")
	requireSynced(t, stringColOf(t, checkDB, "SELECT parent_name AS col FROM sys_menu WHERE id = ?", childID), "菜单甲", "改名前子菜单 sys_menu.parent_name 基线")

	// 改名 → 两表同步为新名（menu 事件表 1/2）
	putViaAPI(t, ts, token, "/api/v1/menus/"+strconv.FormatInt(menuID, 10), `{"name":"菜单甲新名"}`)
	requireSynced(t, stringColOf(t, checkDB, "SELECT menu_name AS col FROM sys_role_menu WHERE menu_id = ?", menuID), "菜单甲新名", "改名后 sys_role_menu.menu_name 同步（menu 事件表 1）")
	requireSynced(t, stringColOf(t, checkDB, "SELECT parent_name AS col FROM sys_menu WHERE id = ?", childID), "菜单甲新名", "改名后子菜单 sys_menu.parent_name 同步（menu 事件表 2）")
}

// TestDictTypeNameSync 字典类型改名端到端同步直证（SYNC-01 dictType 事件 1 表）：
// sys_dict_data.dict_type_name（dict_type_id 锚，CreateDictData 服务端查库冗余）。
func TestDictTypeNameSync(t *testing.T) {
	ts, dsn, adminToken, _ := newTestApp(t)
	checkDB := openCheckDB(t, dsn)
	token := adminToken()

	typeID := createViaAPI(t, ts, token, "POST", "/api/v1/dict-types", `{"dictName":"故障类型","dictType":"fault_kind_sync"}`)
	createViaAPI(t, ts, token, "POST", "/api/v1/dict-datas", `{"dictTypeId":"`+strconv.FormatInt(typeID, 10)+`","label":"类型A","value":"A"}`)

	// 基线：dict_data.dict_type_name 为创建时服务端冗余的旧名
	requireSynced(t, stringColOf(t, checkDB, "SELECT dict_type_name AS col FROM sys_dict_data WHERE dict_type_id = ?", typeID), "故障类型", "改名前 sys_dict_data.dict_type_name 基线")

	// 改名 → dict_type_name 同步为新名（dictType 事件表 1）
	putViaAPI(t, ts, token, "/api/v1/dict-types/"+strconv.FormatInt(typeID, 10), `{"dictName":"故障类别"}`)
	requireSynced(t, stringColOf(t, checkDB, "SELECT dict_type_name AS col FROM sys_dict_data WHERE dict_type_id = ?", typeID), "故障类别", "改名后 sys_dict_data.dict_type_name 同步（dictType 事件表 1）")
}
// ==== Task 2 RED:追加到 internal/handler/name_sync_integration_test.go 的内容 ====

// assertCol 断言任意单行单列查询值精确相等(14 表循环断言出口)。
func assertCol(t *testing.T, db *gorm.DB, want, msg string, query string, args ...any) {
	t.Helper()
	requireSynced(t, stringColOf(t, db, query, args...), want, msg)
}

// assertColNull 断言单行单列查询值为 NULL(SYNC-02 NULL 语义锁定出口——禁 IS NULL
// 放行分支的直证:冗余列为 NULL 的行在改名后仍为 NULL,SQL `<>` 三值逻辑恒不中)。
func assertColNull(t *testing.T, db *gorm.DB, msg string, query string, args ...any) {
	t.Helper()
	require.Nil(t, stringColOf(t, db, query, args...), "%s（列值非 NULL）", msg)
}

// TestRealNameSync14Tables 用户改真名 → 14 张表冗余列全部同步为新真名（SYNC-01
// realName 事件全量面，26 条 UPDATE 逐表断言非抽样）：sys_user 双审计列、
// sys_user_role.username（**值为新真名——展示名语义**，源命名错位原样移植）、
// 11 张 BaseWithOperator 域表 create_by_name/update_by_name、
// sys_operation_log.real_name（user_id 锚独立业务列）。锚用户 = 种子 admin
// （realName='管理员'）——API 建行即以 admin 为操作人；admin 未触达的表行 SQL 直插
// 补齐（仅 id 非空 DDL，审计四列显式基线）。
func TestRealNameSync14Tables(t *testing.T) {
	ts, dsn, adminToken, adminID := newTestApp(t)
	checkDB := openCheckDB(t, dsn)
	token := adminToken()

	// sys_user 行素材：经 API 建（真实链路）后显式覆写审计四列为确定基线
	deptID := createViaAPI(t, ts, token, "POST", "/api/v1/depts", `{"deptName":"审计锚点部"}`)
	u2Str := createRawUser(t, ts, token, `{"username":"rn-anchor","password":"pw-123456","realName":"审计锚点","deptId":"`+strconv.FormatInt(deptID, 10)+`"}`)
	u2, err := strconv.ParseInt(u2Str, 10, 64)
	require.NoError(t, err, "user id 必须是十进制字符串: %q", u2Str)
	execMust(t, checkDB, "UPDATE sys_user SET create_by_id = ?, create_by_name = '管理员', update_by_id = ?, update_by_name = '管理员' WHERE id = ?", adminID, adminID, u2)

	// 11 张 BaseWithOperator 域表：显式审计四列直插（旧行名='管理员'）
	opTables := []struct {
		table string
		id    int64
	}{
		{"sys_dept", 7001}, {"sys_role", 7002}, {"sys_menu", 7003}, {"sys_dict_type", 7004},
		{"sys_dict_data", 7005}, {"sys_config", 7006}, {"sys_api_key", 7007}, {"sys_shift", 7008},
		{"sys_fault_info", 7009}, {"sys_file_category", 7010}, {"sys_file", 7011},
	}
	for _, s := range opTables {
		execMust(t, checkDB,
			"INSERT INTO "+s.table+" (id, create_by_id, create_by_name, update_by_id, update_by_name) VALUES (?, ?, '管理员', ?, '管理员')",
			s.id, adminID, adminID)
	}
	// sys_user_role 行（种子 attachRoleToUser 已建 user_id=adminID 行）：username 基线
	execMust(t, checkDB, "UPDATE sys_user_role SET username = '管理员' WHERE user_id = ?", adminID)
	// sys_operation_log 行：user_id 锚 + real_name 独立业务列
	execMust(t, checkDB, "INSERT INTO sys_operation_log (id, user_id, real_name) VALUES (?, ?, '管理员')", 7012, adminID)

	// 改真名 → 14 表 26 列全部 = 新真名
	putViaAPI(t, ts, token, "/api/v1/users/"+strconv.FormatInt(adminID, 10), `{"realName":"管理员甲"}`)

	for _, s := range opTables {
		assertCol(t, checkDB, "管理员甲", "改名后 "+s.table+".create_by_name 同步（realName 事件表行）",
			"SELECT create_by_name AS col FROM "+s.table+" WHERE id = ?", s.id)
		assertCol(t, checkDB, "管理员甲", "改名后 "+s.table+".update_by_name 同步（realName 事件表行）",
			"SELECT update_by_name AS col FROM "+s.table+" WHERE id = ?", s.id)
	}
	assertCol(t, checkDB, "管理员甲", "改名后 sys_user.create_by_name 同步（表 1/14）",
		"SELECT create_by_name AS col FROM sys_user WHERE id = ?", u2)
	assertCol(t, checkDB, "管理员甲", "改名后 sys_user.update_by_name 同步（表 1/14）",
		"SELECT update_by_name AS col FROM sys_user WHERE id = ?", u2)
	// 列名 username 实存展示名（源 UserApplicationService 命名错位原样移植，勿"修正"）
	assertCol(t, checkDB, "管理员甲", "改名后 sys_user_role.username 同步（表 2/14——列名 username、值=展示名 realName，源命名错位语义）",
		"SELECT username AS col FROM sys_user_role WHERE user_id = ?", adminID)
	assertCol(t, checkDB, "管理员甲", "改名后 sys_operation_log.real_name 同步（表 14/14，user_id 锚）",
		"SELECT real_name AS col FROM sys_operation_log WHERE id = ?", 7012)
}

// TestNullNameRowsNotSynced NULL 语义锁定（SYNC-02 prohibition 直证）：冗余列为
// NULL 的行在改名后仍为 NULL——SQL `<>` 对 NULL 比较恒不中，禁 IS NULL 放行分支。
// 每域配一行非 NULL 控制行：控制行被同步（证明改名确实生效）而 NULL 行保持 NULL。
func TestNullNameRowsNotSynced(t *testing.T) {
	ts, dsn, adminToken, adminID := newTestApp(t)
	checkDB := openCheckDB(t, dsn)
	token := adminToken()

	deptID := createViaAPI(t, ts, token, "POST", "/api/v1/depts", `{"deptName":"空值部门"}`)
	typeID := createViaAPI(t, ts, token, "POST", "/api/v1/dict-types", `{"dictName":"空值类型","dictType":"null_kind"}`)

	// 四类 NULL 冗余行 + 各域非 NULL 控制行（SQL 直插）
	execMust(t, checkDB, "INSERT INTO sys_dept (id, parent_id, dept_name, sort, status) VALUES (?, ?, '空冗余子部门', 0, 1)", 7101, deptID)
	execMust(t, checkDB, "INSERT INTO sys_dept (id, parent_id, parent_name, dept_name, sort, status) VALUES (?, ?, '空值部门', '控制子部门', 0, 1)", 7103, deptID)
	execMust(t, checkDB, "INSERT INTO sys_user (id, username, password, real_name, dept_id, status, builtin) VALUES (?, 'null-dept-user', 'x', '空冗余用户', ?, 1, 0)", 7102, deptID)
	execMust(t, checkDB, "INSERT INTO sys_dict_data (id, dict_type_id, label, value) VALUES (?, ?, '空类型A', 'A')", 7104, typeID)
	execMust(t, checkDB, "INSERT INTO sys_dict_data (id, dict_type_id, dict_type_name, label, value) VALUES (?, ?, '空值类型', '控制B', 'B')", 7105, typeID)
	execMust(t, checkDB, "INSERT INTO sys_operation_log (id, user_id) VALUES (?, ?)", 7106, adminID)
	execMust(t, checkDB, "INSERT INTO sys_operation_log (id, user_id, real_name) VALUES (?, ?, '管理员')", 7107, adminID)

	// 触发三域改名（dept/dictType/realName）
	putViaAPI(t, ts, token, "/api/v1/depts/"+strconv.FormatInt(deptID, 10), `{"deptName":"空值部门新名"}`)
	putViaAPI(t, ts, token, "/api/v1/dict-types/"+strconv.FormatInt(typeID, 10), `{"dictName":"空值类型新名"}`)
	putViaAPI(t, ts, token, "/api/v1/users/"+strconv.FormatInt(adminID, 10), `{"realName":"管理员甲"}`)

	// NULL 行保持 NULL（`<>` 恒不中）
	assertColNull(t, checkDB, "dept 改名后 sys_dept.parent_name 保持 NULL（NULL 行 `<>` 恒不中）",
		"SELECT parent_name AS col FROM sys_dept WHERE id = ?", 7101)
	assertColNull(t, checkDB, "dept 改名后 sys_user.dept_name 保持 NULL（NULL 行 `<>` 恒不中）",
		"SELECT dept_name AS col FROM sys_user WHERE id = ?", 7102)
	assertColNull(t, checkDB, "dictType 改名后 sys_dict_data.dict_type_name 保持 NULL（NULL 行 `<>` 恒不中）",
		"SELECT dict_type_name AS col FROM sys_dict_data WHERE id = ?", 7104)
	assertColNull(t, checkDB, "realName 改名后 sys_operation_log.real_name 保持 NULL（NULL 行 `<>` 恒不中）",
		"SELECT real_name AS col FROM sys_operation_log WHERE id = ?", 7106)

	// 非 NULL 控制行同步（证明改名与同步链路确实生效——NULL 行是"被跳过"非"全没跑"）
	assertCol(t, checkDB, "空值部门新名", "控制行 sys_dept.parent_name 已同步",
		"SELECT parent_name AS col FROM sys_dept WHERE id = ?", 7103)
	assertCol(t, checkDB, "空值类型新名", "控制行 sys_dict_data.dict_type_name 已同步",
		"SELECT dict_type_name AS col FROM sys_dict_data WHERE id = ?", 7105)
	assertCol(t, checkDB, "管理员甲", "控制行 sys_operation_log.real_name 已同步",
		"SELECT real_name AS col FROM sys_operation_log WHERE id = ?", 7107)
}

// TestIdempotentSelfHeal 幂等自愈重放（SYNC-02，realName 轴直证）：①有效改名同步；
// ②手工把已同步行改回陈旧值 → 再一次有效改名（旧名→另一新名）→ `<>` 命中，陈旧行
// 被拉齐到最新名（跳跃自愈）；③对已同步行直调 handler 重放同一事件 → `<>` 不中，
// 该行值不变。
func TestIdempotentSelfHeal(t *testing.T) {
	ts, dsn, adminToken, adminID := newTestApp(t)
	checkDB := openCheckDB(t, dsn)
	token := adminToken()

	// realName 轴素材：sys_dept 行 create_by_id=adminID（旧行名='管理员'）
	execMust(t, checkDB, "INSERT INTO sys_dept (id, dept_name, sort, status, create_by_id, create_by_name) VALUES (?, '自愈部门', 0, 1, ?, '管理员')", 7201, adminID)

	// ①有效改名 → 行同步
	putViaAPI(t, ts, token, "/api/v1/users/"+strconv.FormatInt(adminID, 10), `{"realName":"管理员甲"}`)
	assertCol(t, checkDB, "管理员甲", "首次改名后 create_by_name 同步基线",
		"SELECT create_by_name AS col FROM sys_dept WHERE id = ?", 7201)

	// ②手工制造陈旧行 → 再一次有效改名 → `<>` 命中拉齐（自愈直证）
	execMust(t, checkDB, "UPDATE sys_dept SET create_by_name = '陈旧名' WHERE id = ?", 7201)
	putViaAPI(t, ts, token, "/api/v1/users/"+strconv.FormatInt(adminID, 10), `{"realName":"管理员乙"}`)
	assertCol(t, checkDB, "管理员乙", "陈旧行被 `<>` 拉齐到最新名（幂等自愈）",
		"SELECT create_by_name AS col FROM sys_dept WHERE id = ?", 7201)

	// ③已同步行直调 handler 重放同一事件 → `<>` 不中，值不变
	h := event.NewSyncHandlers(checkDB)
	require.NoError(t, h.OnUserRealNameChanged(context.Background(), event.NameChangedEvent{
		Type: event.RealNameChanged, ID: adminID, OldName: "管理员乙", NewName: "管理员乙",
	}), "handler 直调重放不得报错")
	assertCol(t, checkDB, "管理员乙", "已同步行重放同一事件后值不变（`<>` 不中，零 UPDATE 生效）",
		"SELECT create_by_name AS col FROM sys_dept WHERE id = ?", 7201)
}
