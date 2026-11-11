// sys_role_menu（角色-菜单关联）数据访问：casbin p 行对账原料（RolePermRule）
// 与单角色恢复重建原料、assignMenus 全删重建、role 删除级联清理。
package repository

import (
	"context"

	"github.com/cool-wx/sys-iam-go/internal/model/po"
)

// RolePermRule boot 对账投影（casbin p 行原料）：role_code → perm 串。
type RolePermRule struct {
	RoleCode string
	Perm     string
}

// FindRolePerms boot 对账数据源二（casbin p 行导入）：sys_role_menu rm JOIN
// sys_role r（取 role_code）JOIN sys_menu m（取 permission 串），仅投影非空
// permission；停用角色不导入（与 FindUserRoleCodes 同口径——停用角色的
// p 行运行期已随 Update 停用 ClearRole，对账不得复活）；内存 map 去重（Error 1869
// 防御第一道——MySQL 8.4 同批重复行整批失败，03-RESEARCH probe 实证）。返回恒非 nil。
func (r *RelationRepo) FindRolePerms(ctx context.Context) ([]RolePermRule, error) {
	var rows []RolePermRule
	err := r.db.WithContext(ctx).
		Table("sys_role_menu AS rm").
		Select("r.role_code, m.permission AS perm").
		Joins("JOIN sys_role r ON rm.role_id = r.id AND r.status = 1").
		Joins("JOIN sys_menu m ON rm.menu_id = m.id").
		Where("m.permission IS NOT NULL AND m.permission != ''").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]RolePermRule, 0, len(rows))
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		key := row.RoleCode + "\x00" + row.Perm
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, row)
	}
	return out, nil
}

// FindMenuIDsByRoleID 返回角色已分配的 menuId 列表；
// 未分配返回空切片（恒非 nil）。id 升序输出保证稳定。
func (r *RelationRepo) FindMenuIDsByRoleID(ctx context.Context, roleID int64) (menuIDs []int64, err error) {
	err = r.db.WithContext(ctx).Model(&po.RoleMenuPO{}).
		Where("role_id = ?", roleID).
		Order("menu_id ASC").
		Pluck("menu_id", &menuIDs).Error
	if err != nil {
		return nil, err
	}
	if menuIDs == nil {
		menuIDs = make([]int64, 0)
	}
	return menuIDs, nil
}

// DeleteRoleMenus 删除角色的全部菜单关联（assignMenus 全删重建的前半步 /
// role 删除四连删的第二步）；roleID 经参数化占位符传入。
func (r *RelationRepo) DeleteRoleMenus(ctx context.Context, roleID int64) (err error) {
	return r.db.WithContext(ctx).Where("role_id = ?", roleID).Delete(&po.RoleMenuPO{}).Error
}

// BatchCreateRoleMenus 批量插入角色-菜单关联（assignMenus 全删重建的后半步）；
// 行内 role_name/menu_name 冗余列由 service 层装配。
func (r *RelationRepo) BatchCreateRoleMenus(ctx context.Context, rows []po.RoleMenuPO) (err error) {
	if len(rows) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).CreateInBatches(rows, 100).Error
}

// FindPermsByRoleID 返回角色经 sys_role_menu 关联的全部非空 permission 串（升序、
// map 去重——Error 1869 防御同 FindRolePerms）：角色恢复 ENABLED 时 SyncRolePerms
// 重建 p 行的原料（与 boot 对账数据源二同口径，仅限定单角色）。返回恒非 nil。
func (r *RelationRepo) FindPermsByRoleID(ctx context.Context, roleID int64) ([]string, error) {
	var rows []string
	err := r.db.WithContext(ctx).
		Table("sys_role_menu AS rm").
		Select("DISTINCT m.permission AS perm").
		Joins("JOIN sys_menu m ON rm.menu_id = m.id").
		Where("rm.role_id = ? AND m.permission IS NOT NULL AND m.permission != ''", roleID).
		Order("perm ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	if rows == nil {
		rows = make([]string, 0)
	}
	return rows, nil
}
