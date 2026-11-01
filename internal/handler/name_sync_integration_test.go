//go:build integration

package handler_test

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestDeptNameSync 部门改名端到端同步直证（Phase 5 SYNC-01/02 tracer，05-01 Task 2；
// 05-02 扩展同一文件覆盖其余 4 域）。链路：PUT 改名 → service 提交后发布
// DeptNameChanged → dispatcher 同步分发 → handler 独立事务幂等 UPDATE →
// sys_user.dept_name（dept_id 锚）与 sys_dept.parent_name（parent_id 锚）可断言。
// 覆盖五语义：Create 自动冗余基线、改名双表同步正例、参数化特殊字符（T-5-01）、
// 仅改备注零同步（发布条件负例）、幂等重放自愈 + 同名重放零 UPDATE。
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

	// --- 同名重放：新名==旧行名不发布 → 零同步 UPDATE（哨兵值保持直证）---
	execMust(t, checkDB, "UPDATE sys_user SET dept_name = ? WHERE id = ?", "哨兵值", userID)
	status, body = putDept(deptA, `{"deptName":"研发三部"}`)
	require.Equal(t, http.StatusOK, status, "同名重放失败: %s", body)
	requireSynced(t, userDeptNameOf(t, checkDB, userID), "哨兵值",
		"同名 PUT 不发布事件——sys_user.dept_name 必须保持哨兵值（零同步 UPDATE）")
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
