package po

import (
	"time"

	"gorm.io/gorm"

	"github.com/cool-wx/sys-iam-go/internal/auth"
	"github.com/cool-wx/sys-iam-go/pkg/ids"
)

// DeptPO sys_dept 表映射（13 列，列名对齐真实 DDL scripts/ddl/sys_dept.sql，
// snake_case）。ID 由应用层雪花生成（DB 无自增）；审计列由 BeforeCreate 从
// ctx identity 填充。树列 parent_id 用 *int64（NULL=根；0 亦按根处理，源语义）。
type DeptPO struct {
	BaseWithOperator

	ID         int64   `gorm:"column:id;primaryKey"`
	ParentID   *int64  `gorm:"column:parent_id"`
	ParentName *string `gorm:"column:parent_name"`
	DeptName   *string `gorm:"column:dept_name"`
	Sort       *int    `gorm:"column:sort"`
	Status     *int8   `gorm:"column:status"`
}

// TableName 显式表名（Tabler 接口）。
func (DeptPO) TableName() string {
	return "sys_dept"
}

// BeforeCreate 生成雪花 ID（若未预置）+ 填充双时间 + 从 ctx identity 填充四操作人列。
// 无 identity 上下文时操作人列保持 NULL（等价源 AutoFillHandler "系统行为无操作人"）。
func (d *DeptPO) BeforeCreate(tx *gorm.DB) error {
	if d.ID == 0 {
		d.ID = int64(ids.NewID())
	}
	now := time.Now()
	d.CreateTime = &now
	d.UpdateTime = &now
	if id, ok := auth.FromContext(tx.Statement.Context); ok {
		d.CreateByID = &id.UserID
		d.CreateByName = &id.Name
		d.UpdateByID = &id.UserID
		d.UpdateByName = &id.Name
	}
	return nil
}
