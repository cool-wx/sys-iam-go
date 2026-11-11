// 关联表数据访问：RelationRepo 单一结构承载三张关联表，按表拆文件——
// sys_user_role（用户-角色）见 relation_user_role.go、sys_role_menu（角色-菜单）
// 见 relation_role_menu.go、sys_role_dept（角色-范围部门）见 relation_role_dept.go；
// 结构定义与构造集中在本文件。
package repository

import (
	"gorm.io/gorm"
)

// RelationRepo 关联表数据访问（只读最小面；写路径随 Wave 2/3 角色分配需求扩展）。
type RelationRepo struct {
	db *gorm.DB
}

// NewRelationRepo 构造（db 可为 *gorm.DB 或事务 tx）。
func NewRelationRepo(db *gorm.DB) *RelationRepo {
	return &RelationRepo{db: db}
}
