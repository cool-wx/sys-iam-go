package po

import (
	"time"

	"gorm.io/gorm"

	"github.com/cool-wx/sys-iam-go/internal/auth"
	"github.com/cool-wx/sys-iam-go/pkg/ids"
)

// RolePO sys_role 表映射（13 列，列名对齐真实 DDL，snake_case）。
// ID 由应用层雪花生成（DB 无自增）；审计列由 BeforeCreate 从 ctx identity 填充。
type RolePO struct {
	BaseWithOperator

	ID       int64  `gorm:"column:id;primaryKey"`
	RoleName string `gorm:"column:role_name"`
	RoleCode string `gorm:"column:role_code"`
	// DataScope 数据权限档位（源 create 默认 1；存量 super_admin 行为 NULL——指针列
	// 精确保留 NULL 与非 NULL 差异）。
	DataScope *int8 `gorm:"column:data_scope"`
	Status    *int8 `gorm:"column:status"`
	Builtin   *bool `gorm:"column:builtin"`
}

// TableName 显式表名（Tabler 接口）。
func (RolePO) TableName() string {
	return "sys_role"
}

// BeforeCreate 生成雪花 ID（若未预置）+ 填充双时间 + 从 ctx identity 填充四操作人列。
// 无 identity 上下文时操作人列保持 NULL（等价源 AutoFillHandler "系统行为无操作人"）。
func (r *RolePO) BeforeCreate(tx *gorm.DB) error {
	if r.ID == 0 {
		r.ID = int64(ids.NewID())
	}
	now := time.Now()
	r.CreateTime = &now
	r.UpdateTime = &now
	if id, ok := auth.FromContext(tx.Statement.Context); ok {
		r.CreateByID = &id.UserID
		r.CreateByName = &id.Name
		r.UpdateByID = &id.UserID
		r.UpdateByName = &id.Name
	}
	return nil
}
