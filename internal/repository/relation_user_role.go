// sys_user_role（用户-角色关联）数据访问：角色装配投影（RoleRef）、datascope
// 档位投影（RoleScope）、casbin g 行对账原料（UserRule）与恢复重建原料、
// AssignRoles 全删重建、user/role 删除级联清理。
// RelationRepo 单一结构承载三张关联表，按表拆文件——结构定义与构造在本文件；
// 另两张表：sys_role_menu（角色-菜单）见 relation_role_menu.go、
// sys_role_dept（角色-范围部门）见 relation_role_dept.go。
package repository

import (
	"context"

	"gorm.io/gorm"

	"github.com/cool-wx/sys-iam-go/internal/model/po"
)

// RelationRepo 关联表数据访问（只读最小面；写路径随 Wave 2/3 角色分配需求扩展）。
type RelationRepo struct {
	db *gorm.DB
}

// NewRelationRepo 构造（db 可为 *gorm.DB 或事务 tx）。
func NewRelationRepo(db *gorm.DB) *RelationRepo {
	return &RelationRepo{db: db}
}

// RoleRef 角色引用（JOIN 投影）：登录会话 roles 装配与用户详情角色装配共用。
type RoleRef struct {
	RoleID   int64
	RoleName string
	RoleCode string
}

// UserRule boot 对账投影（casbin g 行原料）：user → role_code。
type UserRule struct {
	UserID   int64
	RoleCode string
}

// RoleScope 用户角色档位投影（datascope Resolve 数据源）：role_id +
// data_scope 指针列——NULL 精确保留（存量行为 NULL 的角色不贡献档位，Widest 跳过）。
type RoleScope struct {
	RoleID    int64
	DataScope *int8
}

// FindRolesByUserIDs 一次 IN 查询返回 user→roles 映射（两查询防 N+1 的第一查询，
// 会话装配共用）。返回 map 恒非 nil（无角色用户缺席即可）；行内
// role 行已被 JOIN 过滤（不存在的 role_id 不产生 RoleRef）；停用角色不产生行
// （r.status=1——停用角色对登录装配/详情装配即刻零效果，与用户停用
// 家族语义对称）。
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
		Joins("JOIN sys_role r ON ur.role_id = r.id AND r.status = 1").
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

// FindRoleScopesByUserIDs 一次 IN 查询返回 user→(roleID, data_scope) 映射（照
// FindRolesByUserIDs JOIN 模板）：datascope 档位聚合（Widest）与档位 4 角色部门
// 集查询（FindDeptIDsByRoleIDs）的单一数据源（service Resolve 挂接）。
// 行内 role 行已被 JOIN 过滤（不存在的 role_id 不产生行）；停用角色不产生行
// （停用角色的档位不参与聚合，否则其 data_scope 仍贡献行过滤面）。返回
// map 恒非 nil（无角色用户缺席即可）。
func (r *RelationRepo) FindRoleScopesByUserIDs(ctx context.Context, userIDs []int64) (map[int64][]RoleScope, error) {
	out := make(map[int64][]RoleScope, len(userIDs))
	if len(userIDs) == 0 {
		return out, nil
	}
	var rows []struct {
		UserID    int64
		RoleID    int64
		DataScope *int8
	}
	err := r.db.WithContext(ctx).
		Table("sys_user_role AS ur").
		Select("ur.user_id, r.id AS role_id, r.data_scope").
		Joins("JOIN sys_role r ON ur.role_id = r.id AND r.status = 1").
		Where("ur.user_id IN ?", userIDs).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.UserID] = append(out[row.UserID], RoleScope{RoleID: row.RoleID, DataScope: row.DataScope})
	}
	return out, nil
}

// FindUserRoleCodes boot 对账数据源一（casbin g 行导入）：sys_user_role
// ur JOIN sys_role r 投影 user_id + role_code（照 FindRolesByUserIDs JOIN 模板）。
// 行内 role 行已被 JOIN 过滤（不存在的 role_id 不产生行）；停用角色不导入（
// 否则停用角色的 g 行在重启后复活——运行期 Update 停用已 ClearRole，boot 对账必须
// 同口径，且停用 super_admin 的豁免 g 行绝不可经对账还魂）。返回恒非 nil。
func (r *RelationRepo) FindUserRoleCodes(ctx context.Context) ([]UserRule, error) {
	var rows []struct {
		UserID   int64
		RoleCode string
	}
	err := r.db.WithContext(ctx).
		Table("sys_user_role AS ur").
		Select("ur.user_id, r.role_code").
		Joins("JOIN sys_role r ON ur.role_id = r.id AND r.status = 1").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]UserRule, 0, len(rows))
	for _, row := range rows {
		out = append(out, UserRule{UserID: row.UserID, RoleCode: row.RoleCode})
	}
	return out, nil
}

// DeleteUserRolesByRoleID 删除角色在 sys_user_role 的全部关联
// （role 删除四连删的第一步）。
func (r *RelationRepo) DeleteUserRolesByRoleID(ctx context.Context, roleID int64) (err error) {
	return r.db.WithContext(ctx).Where("role_id = ?", roleID).Delete(&po.UserRolePO{}).Error
}

// DeleteUserRolesByUserID 删除用户的全部角色关联（AssignRoles 全删重建的前半步 /
// user 删除事务的第一步——先删关联再删用户）；userID 经参数化
// 占位符传入。可接事务 tx（NewRelationRepo(tx)）。
func (r *RelationRepo) DeleteUserRolesByUserID(ctx context.Context, userID int64) (err error) {
	return r.db.WithContext(ctx).Where("user_id = ?", userID).Delete(&po.UserRolePO{}).Error
}

// BatchCreateUserRoles 批量插入用户-角色关联（AssignRoles 全删重建的后半步）；
// 行内 username/role_name 冗余列由 service 层装配——
// username 列实存 realName 优先回退 username 的值，命名与内容不符，
// 系既有库列约定，勿修正。
func (r *RelationRepo) BatchCreateUserRoles(ctx context.Context, rows []po.UserRolePO) (err error) {
	if len(rows) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).CreateInBatches(rows, 100).Error
}

// FindUserIDsByRoleID 返回角色在 sys_user_role 的全部关联 userID（升序）：角色
// 恢复 ENABLED 时 SyncRoleUsers 重建 g 行的原料（与 boot 对账数据源一同口径——
// 不按用户 status 过滤，用户轴停用已有踢会话闭环，g 行留存无害）。返回恒非 nil。
func (r *RelationRepo) FindUserIDsByRoleID(ctx context.Context, roleID int64) ([]int64, error) {
	var out []int64
	err := r.db.WithContext(ctx).
		Table("sys_user_role AS ur").
		Select("DISTINCT ur.user_id").
		Where("ur.role_id = ?", roleID).
		Order("ur.user_id ASC").
		Scan(&out).Error
	if err != nil {
		return nil, err
	}
	if out == nil {
		out = make([]int64, 0)
	}
	return out, nil
}
