package po

import (
	"time"

	"gorm.io/gorm"

	"github.com/cool-wx/sys-iam-go/internal/auth"
	"github.com/cool-wx/sys-iam-go/pkg/ids"
)

// ShiftPO sys_shift 表映射（13 列，列名对齐真实 DDL，snake_case）。
// start_time/end_time 为 MySQL TIME 列——以 HH:MM:SS 字符串直传
// （不加 time.Time 解析层、不加自定义格式校验；非法串由 DB 报错走既有翻译
// 路径）。无唯一索引。
type ShiftPO struct {
	BaseWithOperator

	ID        int64   `gorm:"column:id;primaryKey"`
	ShiftCode *string `gorm:"column:shift_code"`
	ShiftName *string `gorm:"column:shift_name"`
	StartTime *string `gorm:"column:start_time"`
	EndTime   *string `gorm:"column:end_time"`
	Status    *int8   `gorm:"column:status"`
}

// TableName 显式表名（Tabler 接口）。
func (ShiftPO) TableName() string {
	return "sys_shift"
}

// BeforeCreate 生成雪花 ID + 双时间 + ctx identity 四操作人填充（同构模板）。
func (s *ShiftPO) BeforeCreate(tx *gorm.DB) error {
	if s.ID == 0 {
		s.ID = int64(ids.NewID())
	}
	now := time.Now()
	s.CreateTime = &now
	s.UpdateTime = &now
	if id, ok := auth.FromContext(tx.Statement.Context); ok {
		s.CreateByID = &id.UserID
		s.CreateByName = &id.Name
		s.UpdateByID = &id.UserID
		s.UpdateByName = &id.Name
	}
	return nil
}
