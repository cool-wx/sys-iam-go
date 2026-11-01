// 关联表数据访问（sys_user_role/sys_role_menu）。
package repository

import (
	"context"

	"gorm.io/gorm"

	"github.com/cool-wx/sys-iam-go/internal/model/po"
)

// RoleRef 角色引用（JOIN 投影）：登录会话 roles 装配与用户详情角色装配共用。
type RoleRef struct {
	RoleID   int64
	RoleName string
	RoleCode string
}

// UserRule boot 对账投影（casbin g 行原料，T-3-05）：user → role_code。
type UserRule struct {
	UserID   int64
	RoleCode string
}

// RolePermRule boot 对账投影（casbin p 行原料）：role_code → perm 串。
type RolePermRule struct {
	RoleCode string
	Perm     string
}

// RoleScope 用户角色档位投影（datascope Resolve 数据源，PERM-03）：role_id +
// data_scope 指针列——NULL 精确保留（存量行为 NULL 的角色不贡献档位，Widest 跳过）。
type RoleScope struct {
	RoleID    int64
	DataScope *int8
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
// role 行已被 JOIN 过滤（不存在的 role_id 不产生 RoleRef）；停用角色不产生行
// （WR-04：r.status=1——停用角色对登录装配/详情装配即刻零效果，与 D-09 用户停用
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
// 集查询（FindDeptIDsByRoleIDs）的单一数据源（service Resolve 挂接，PERM-03）。
// 行内 role 行已被 JOIN 过滤（不存在的 role_id 不产生行）；停用角色不产生行
// （WR-04：停用角色的档位不参与聚合，否则其 data_scope 仍贡献行过滤面）。返回
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

// FindUserRoleCodes boot 对账数据源一（casbin g 行导入，T-3-05）：sys_user_role
// ur JOIN sys_role r 投影 user_id + role_code（照 FindRolesByUserIDs JOIN 模板）。
// 行内 role 行已被 JOIN 过滤（不存在的 role_id 不产生行）；停用角色不导入（WR-04：
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

// FindRolePerms boot 对账数据源二（casbin p 行导入）：sys_role_menu rm JOIN
// sys_role r（取 role_code）JOIN sys_menu m（取 permission 串），仅投影非空
// permission；停用角色不导入（WR-04：与 FindUserRoleCodes 同口径——停用角色的
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
// role 删除四连删的第二步）；roleID 经参数化占位符传入（T-1-03）。
func (r *RelationRepo) DeleteRoleMenus(ctx context.Context, roleID int64) (err error) {
	return r.db.WithContext(ctx).Where("role_id = ?", roleID).Delete(&po.RoleMenuPO{}).Error
}

// DeleteUserRolesByRoleID 删除角色在 sys_user_role 的全部关联
// （role 删除四连删的第一步，源 RoleApplicationService.removeById 顺序）。
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

// DeleteUserRolesByUserID 删除用户的全部角色关联（AssignRoles 全删重建的前半步 /
// user 删除事务的第一步——源 removeById 先删关联再删用户顺序）；userID 经参数化
// 占位符传入（T-1-03）。可接事务 tx（NewRelationRepo(tx)）。
func (r *RelationRepo) DeleteUserRolesByUserID(ctx context.Context, userID int64) (err error) {
	return r.db.WithContext(ctx).Where("user_id = ?", userID).Delete(&po.UserRolePO{}).Error
}

// BatchCreateUserRoles 批量插入用户-角色关联（AssignRoles 全删重建的后半步）；
// 行内 username/role_name 冗余列由 service 层装配（源 UserRole 冗余写语义——
// username 列实存 realName 优先回退 username 的值，命名与内容不符原样移植）。
func (r *RelationRepo) BatchCreateUserRoles(ctx context.Context, rows []po.UserRolePO) (err error) {
	if len(rows) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).CreateInBatches(rows, 100).Error
}

// ---- sys_role_dept（dataScope 档位 4「指定部门」集合存储，D-10——与 RoleMenu
// 三方法完全同构；四方法均经占位符参数化，T-1-03）----

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
// 前半步 / role 删除四连删的第三步，D-10）；roleID 经参数化占位符传入（T-1-03）。
func (r *RelationRepo) DeleteRoleDepts(ctx context.Context, roleID int64) (err error) {
	return r.db.WithContext(ctx).Where("role_id = ?", roleID).Delete(&po.RoleDeptPO{}).Error
}

// BatchCreateRoleDepts 批量插入角色-范围部门关联（AssignScopes 全删重建的后半步）；
// 行内 role_name/dept_name 冗余列由 service 层装配（查库值非请求体，T-2-15）。
func (r *RelationRepo) BatchCreateRoleDepts(ctx context.Context, rows []po.RoleDeptPO) (err error) {
	if len(rows) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).CreateInBatches(rows, 100).Error
}

// FindDeptIDsByRoleIDs 一次 IN 查询返回 role→deptIDs 映射（照 FindRolesByUserIDs
// 映射模板）：dataScope 档位 4 多角色部门集合并集的数据源（service Resolve 挂接，
// PERM-03）。返回 map 恒非 nil（无配置角色缺席即可）。
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

// ---- 角色策略重建原料（WR-04：角色停用/恢复的运行期策略同步）----

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
