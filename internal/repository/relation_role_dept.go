// sys_role_dept（dataScope 档位 4「指定部门」集合存储——与 RoleMenu
// 三方法完全同构；四方法均经占位符参数化）数据访问：AssignScopes 全删重建、
// 档位 4 部门集查询、role 删除级联清理。
package repository

import (
	"context"

	"github.com/cool-wx/sys-iam-go/internal/model/po"
)

// FindDeptIDsByRoleID 返回角色已配置的 deptId 列表（GET /roles/:id/scopes 契约）；
// 未配置返回空切片（恒非 nil）。dept_id 升序输出保证稳定（照 FindMenuIDsByRoleID
// 模板）。
func (r *RelationRepo) FindDeptIDsByRoleID(ctx context.Context, roleID int64) (deptIDs []int64, err error) {
	err = r.db.WithContext(ctx).Model(&po.RoleDeptPO{}).
		Where("role_id = ?", roleID).
		Order("dept_id ASC").
		Pluck("dept_id", &deptIDs).Error
	if err != nil {
		return nil, err
	}
	if deptIDs == nil {
		deptIDs = make([]int64, 0)
	}
	return deptIDs, nil
}

// DeleteRoleDepts 删除角色的全部数据权限范围部门关联（AssignScopes 全删重建的
// 前半步 / role 删除四连删的第三步）；roleID 经参数化占位符传入。
func (r *RelationRepo) DeleteRoleDepts(ctx context.Context, roleID int64) (err error) {
	return r.db.WithContext(ctx).Where("role_id = ?", roleID).Delete(&po.RoleDeptPO{}).Error
}

// BatchCreateRoleDepts 批量插入角色-范围部门关联（AssignScopes 全删重建的后半步）；
// 行内 role_name/dept_name 冗余列由 service 层装配（查库值非请求体）。
func (r *RelationRepo) BatchCreateRoleDepts(ctx context.Context, rows []po.RoleDeptPO) (err error) {
	if len(rows) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).CreateInBatches(rows, 100).Error
}

// FindDeptIDsByRoleIDs 一次 IN 查询返回 role→deptIDs 映射（照 FindRolesByUserIDs
// 映射模板）：dataScope 档位 4 多角色部门集合并集的数据源（service Resolve 挂接）。
// 返回 map 恒非 nil（无配置角色缺席即可）。
func (r *RelationRepo) FindDeptIDsByRoleIDs(ctx context.Context, roleIDs []int64) (map[int64][]int64, error) {
	out := make(map[int64][]int64, len(roleIDs))
	if len(roleIDs) == 0 {
		return out, nil
	}
	var rows []struct {
		RoleID int64
		DeptID int64
	}
	err := r.db.WithContext(ctx).
		Table("sys_role_dept AS rd").
		Select("rd.role_id, rd.dept_id").
		Where("rd.role_id IN ?", roleIDs).
		Order("rd.dept_id ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.RoleID] = append(out[row.RoleID], row.DeptID)
	}
	return out, nil
}
