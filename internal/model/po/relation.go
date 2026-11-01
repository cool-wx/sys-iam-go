package po

import (
	"time"

	"gorm.io/gorm"

	"github.com/cool-wx/sys-iam-go/pkg/ids"
)

// UserRolePO sys_user_role 表映射（8 列，列名对齐真实 DDL）。
// 关联表无审计人四列（DDL 实证，02-RESEARCH 研究重点 2）：不内嵌 BaseWithOperator，
// 仅 id + 业务列 + create_time/update_time + remark；BeforeCreate 只填 ID 与双时间。
type UserRolePO struct {
	ID       int64  `gorm:"column:id;primaryKey"`
	UserID   int64  `gorm:"column:user_id"`
	Username string `gorm:"column:username"`
	RoleID   int64  `gorm:"column:role_id"`
	RoleName string `gorm:"column:role_name"`

	CreateTime *time.Time `gorm:"column:create_time"`
	UpdateTime *time.Time `gorm:"column:update_time"`
	Remark     *string    `gorm:"column:remark"`
}

// TableName 显式表名（Tabler 接口）。
func (UserRolePO) TableName() string {
	return "sys_user_role"
}

// BeforeCreate 生成雪花 ID（若未预置）+ 填充双时间（关联表无操作人列可填）。
func (ur *UserRolePO) BeforeCreate(tx *gorm.DB) error {
	if ur.ID == 0 {
		ur.ID = int64(ids.NewID())
	}
	now := time.Now()
	ur.CreateTime = &now
	ur.UpdateTime = &now
	return nil
}

// RoleMenuPO sys_role_menu 表映射（8 列，列名对齐真实 DDL）。
// 同 UserRolePO：关联表无审计人四列，不内嵌 BaseWithOperator。
type RoleMenuPO struct {
	ID       int64  `gorm:"column:id;primaryKey"`
	RoleID   int64  `gorm:"column:role_id"`
	RoleName string `gorm:"column:role_name"`
	MenuID   int64  `gorm:"column:menu_id"`
	MenuName string `gorm:"column:menu_name"`

	CreateTime *time.Time `gorm:"column:create_time"`
	UpdateTime *time.Time `gorm:"column:update_time"`
	Remark     *string    `gorm:"column:remark"`
}

// TableName 显式表名（Tabler 接口）。
func (RoleMenuPO) TableName() string {
	return "sys_role_menu"
}

// BeforeCreate 生成雪花 ID（若未预置）+ 填充双时间。
func (rm *RoleMenuPO) BeforeCreate(tx *gorm.DB) error {
	if rm.ID == 0 {
		rm.ID = int64(ids.NewID())
	}
	now := time.Now()
	rm.CreateTime = &now
	rm.UpdateTime = &now
	return nil
}

// RoleDeptPO sys_role_dept 表映射（8 列，与 sys_role_menu 完全同构——CONTEXT.md
// D-10 裁决方案 A：dataScope 档位 4「指定部门」集合存储，源系统无此概念属增量
// 扩展例外族）。同 RoleMenuPO：关联表无审计人四列，不内嵌 BaseWithOperator。
type RoleDeptPO struct {
	ID       int64  `gorm:"column:id;primaryKey"`
	RoleID   int64  `gorm:"column:role_id"`
	RoleName string `gorm:"column:role_name"`
	DeptID   int64  `gorm:"column:dept_id"`
	DeptName string `gorm:"column:dept_name"`

	CreateTime *time.Time `gorm:"column:create_time"`
	UpdateTime *time.Time `gorm:"column:update_time"`
	Remark     *string    `gorm:"column:remark"`
}

// TableName 显式表名（Tabler 接口）。
func (RoleDeptPO) TableName() string {
	return "sys_role_dept"
}

// BeforeCreate 生成雪花 ID（若未预置）+ 填充双时间。
func (rd *RoleDeptPO) BeforeCreate(tx *gorm.DB) error {
	if rd.ID == 0 {
		rd.ID = int64(ids.NewID())
	}
	now := time.Now()
	rd.CreateTime = &now
	rd.UpdateTime = &now
	return nil
}
