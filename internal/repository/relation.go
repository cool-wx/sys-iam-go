// 关联表数据访问（sys_user_role/sys_role_menu）。
package repository

import (
	"context"

	"gorm.io/gorm"

	"github.com/cool-wx/sys-iam-go/internal/model/po"
)

// RoleRef 角色引用（JOIN 投影）：登录会话 roles 载荷与用户详情角色装配共用。
type RoleRef struct {
	RoleID   int64
	RoleName string
	RoleCode string
}

// RelationRepo 关联表数据访问（只读最小面；写路径随 Wave 2/3 角色分配需求扩展）。
type RelationRepo struct {
	db *gorm.DB
}

// NewRelationRepo 构造（db 可为 *gorm.DB 或事务 tx）。
func NewRelationRepo(db *gorm.DB) *RelationRepo {
	return &RelationRepo{db: db}
}

// FindRolesByUserIDs 一次 IN 查询返回 user→roles 映射（两查询防 N+1 的第一查询，
// USER-05/AUTH-01 会话装配共用）。返回 map 恒非 nil（无角色用户缺席即可）；行内
// role 行已被 JOIN 过滤（不存在的 role_id 不产生 RoleRef）。
func (r *RelationRepo) FindRolesByUserIDs(ctx context.Context, userIDs []int64) (map[int64][]RoleRef, error) {
	out := make(map[int64][]RoleRef, len(userIDs))
	if len(userIDs) == 0 {
		return out, nil
	}
	var rows []struct {
		UserID   int64
		RoleID   int64
		RoleName string
		RoleCode string
	}
	err := r.db.WithContext(ctx).
		Table("sys_user_role AS ur").
		Select("ur.user_id, r.id AS role_id, r.role_name, r.role_code").
		Joins("JOIN sys_role r ON ur.role_id = r.id").
		Where("ur.user_id IN ?", userIDs).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.UserID] = append(out[row.UserID], RoleRef{
			RoleID:   row.RoleID,
			RoleName: row.RoleName,
			RoleCode: row.RoleCode,
		})
	}
	return out, nil
}

// FindMenuIDsByRoleID 返回角色已分配的 menuId 列表（源 getRoleMenuIds 等价）；
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
// role 删除三连删的第二步）；roleID 经参数化占位符传入（T-1-03）。
func (r *RelationRepo) DeleteRoleMenus(ctx context.Context, roleID int64) (err error) {
	return r.db.WithContext(ctx).Where("role_id = ?", roleID).Delete(&po.RoleMenuPO{}).Error
}

// DeleteUserRolesByRoleID 删除角色在 sys_user_role 的全部关联
// （role 删除三连删的第一步，源 RoleApplicationService.removeById 顺序）。
func (r *RelationRepo) DeleteUserRolesByRoleID(ctx context.Context, roleID int64) (err error) {
	return r.db.WithContext(ctx).Where("role_id = ?", roleID).Delete(&po.UserRolePO{}).Error
}

// BatchCreateRoleMenus 批量插入角色-菜单关联（assignMenus 全删重建的后半步）；
// 行内 role_name/menu_name 冗余列由 service 层装配（源 RoleMenu.create 语义）。
func (r *RelationRepo) BatchCreateRoleMenus(ctx context.Context, rows []po.RoleMenuPO) (err error) {
	if len(rows) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).CreateInBatches(rows, 100).Error
}
