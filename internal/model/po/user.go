package po

import (
	"time"

	"gorm.io/gorm"

	"github.com/cool-wx/sys-iam-go/internal/auth"
	"github.com/cool-wx/sys-iam-go/pkg/ids"
)

// UserPO sys_user 表映射（22 列，列名对齐真实 DDL，snake_case）。
// ID 由应用层雪花生成（DB 无自增）；审计列由 BeforeCreate 从 ctx identity 填充。
type UserPO struct {
	BaseWithOperator

	ID         int64   `gorm:"column:id;primaryKey"`
	Username   *string `gorm:"column:username"`
	Password   *string `gorm:"column:password"`
	RealName   *string `gorm:"column:real_name"`
	Avatar     *string `gorm:"column:avatar"`
	Phone      *string `gorm:"column:phone"`
	Email      *string `gorm:"column:email"`
	Openid     *string `gorm:"column:openid"`
	ExtToken   *string `gorm:"column:ext_token"`
	Gender     *int8   `gorm:"column:gender"`
	DeptID     *int64  `gorm:"column:dept_id"`
	DeptName   *string `gorm:"column:dept_name"`
	Status     *int8   `gorm:"column:status"`
	FirstLogin *bool   `gorm:"column:first_login"`
	Builtin    *bool   `gorm:"column:builtin"`
}

// TableName 显式表名（Tabler 接口）。
func (UserPO) TableName() string {
	return "sys_user"
}

// BeforeCreate 生成雪花 ID（若未预置）+ 填充双时间 + 从 ctx identity 填充四操作人列。
// 无 identity 上下文时操作人列保持 NULL（系统行为无操作人）。
func (u *UserPO) BeforeCreate(tx *gorm.DB) error {
	if u.ID == 0 {
		u.ID = int64(ids.NewID())
	}
	now := time.Now()
	u.CreateTime = &now
	u.UpdateTime = &now
	if id, ok := auth.FromContext(tx.Statement.Context); ok {
		u.CreateByID = &id.UserID
		u.CreateByName = &id.Name
		u.UpdateByID = &id.UserID
		u.UpdateByName = &id.Name
	}
	return nil
}
