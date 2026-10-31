// 关联表数据访问（sys_user_role/sys_role_menu）。
package repository

import (
	"context"

	"gorm.io/gorm"
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
