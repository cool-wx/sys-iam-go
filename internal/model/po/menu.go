package po

import (
	"time"

	"gorm.io/gorm"

	"github.com/cool-wx/sys-iam-go/internal/auth"
	"github.com/cool-wx/sys-iam-go/pkg/ids"
)

// MenuPO sys_menu 表映射（19 列，列名对齐真实 DDL，snake_case）。
// ID 由应用层雪花生成（DB 无自增）；审计列由 BeforeCreate 从 ctx identity 填充。
// 树列 parent_id 用 *int64（NULL=根；0 亦按根处理，源 buildTree 语义）。
type MenuPO struct {
	BaseWithOperator

	ID         int64   `gorm:"column:id;primaryKey"`
	ParentID   *int64  `gorm:"column:parent_id"`
	ParentName *string `gorm:"column:parent_name"`
	Name       *string `gorm:"column:name"`
	Type       *int8   `gorm:"column:type"`
	Path       *string `gorm:"column:path"`
	Component  *string `gorm:"column:component"`
	Permission *string `gorm:"column:permission"`
	Icon       *string `gorm:"column:icon"`
	Sort       *int    `gorm:"column:sort"`
	Visible    *bool   `gorm:"column:visible"`
	Status     *int8   `gorm:"column:status"`
}

// TableName 显式表名（Tabler 接口）。
func (MenuPO) TableName() string {
	return "sys_menu"
}

// BeforeCreate 生成雪花 ID（若未预置）+ 填充双时间 + 从 ctx identity 填充四操作人列。
// 无 identity 上下文时操作人列保持 NULL（等价源 AutoFillHandler "系统行为无操作人"）。
func (m *MenuPO) BeforeCreate(tx *gorm.DB) error {
	if m.ID == 0 {
		m.ID = int64(ids.NewID())
	}
	now := time.Now()
	m.CreateTime = &now
	m.UpdateTime = &now
	if id, ok := auth.FromContext(tx.Statement.Context); ok {
		m.CreateByID = &id.UserID
		m.CreateByName = &id.Name
		m.UpdateByID = &id.UserID
		m.UpdateByName = &id.Name
	}
	return nil
}
