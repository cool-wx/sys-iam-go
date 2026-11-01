package po

import (
	"time"

	"gorm.io/gorm"

	"github.com/cool-wx/sys-iam-go/internal/auth"
	"github.com/cool-wx/sys-iam-go/pkg/ids"
)

// FaultInfoPO sys_fault_info 表映射（17 列，列名对齐真实 DDL，snake_case）。
// bit_index/type 的 DB DEFAULT -1/3 仅为 DDL 兜底——GORM nil 指针显式写 NULL 会
// 绕过列默认，服务端默认值由 service 层承担（FAUL-01）。dict_data_id 不进 Excel
// 导入（源 @ExcelProperty 无此列，D-15），仅 CRUD 路径可写。三键
// name+bit_index+location 无唯一索引——判重由应用层查询承担（照源 eq 语义）。
type FaultInfoPO struct {
	BaseWithOperator

	ID         int64   `gorm:"column:id;primaryKey"`
	Code       *string `gorm:"column:code"`
	Name       *string `gorm:"column:name"`
	BitIndex   *int    `gorm:"column:bit_index"`
	Type       *int8   `gorm:"column:type"`
	DictDataID *int64  `gorm:"column:dict_data_id"`
	Detail     *string `gorm:"column:detail"`
	Reason     *string `gorm:"column:reason"`
	Location   *string `gorm:"column:location"`
	Fix        *string `gorm:"column:fix"`
}

// TableName 显式表名（Tabler 接口）。
func (FaultInfoPO) TableName() string {
	return "sys_fault_info"
}

// BeforeCreate 生成雪花 ID + 双时间 + ctx identity 四操作人填充（同构模板）。
func (f *FaultInfoPO) BeforeCreate(tx *gorm.DB) error {
	if f.ID == 0 {
		f.ID = int64(ids.NewID())
	}
	now := time.Now()
	f.CreateTime = &now
	f.UpdateTime = &now
	if id, ok := auth.FromContext(tx.Statement.Context); ok {
		f.CreateByID = &id.UserID
		f.CreateByName = &id.Name
		f.UpdateByID = &id.UserID
		f.UpdateByName = &id.Name
	}
	return nil
}
