package po

import (
	"time"

	"gorm.io/gorm"

	"github.com/cool-wx/sys-iam-go/internal/auth"
	"github.com/cool-wx/sys-iam-go/pkg/ids"
)

// DictTypePO sys_dict_type 表映射（11 列，列名对齐真实 DDL，snake_case）。
// ID 由应用层雪花生成（DB 无自增）；审计列由 BeforeCreate 从 ctx identity 填充。
// dict_type 列由 DB 层 UNIQUE KEY uk_dict_type 约束（判重 1062 语义）。
type DictTypePO struct {
	BaseWithOperator

	ID       int64  `gorm:"column:id;primaryKey"`
	DictName string `gorm:"column:dict_name"`
	DictType string `gorm:"column:dict_type"`
	Status   *int8  `gorm:"column:status"`
}

// TableName 显式表名（Tabler 接口）。
func (DictTypePO) TableName() string {
	return "sys_dict_type"
}

// BeforeCreate 生成雪花 ID（若未预置）+ 填充双时间 + 从 ctx identity 填充四操作人列。
// 无 identity 上下文时操作人列保持 NULL（等价源 AutoFillHandler "系统行为无操作人"）。
// 与 MenuPO.BeforeCreate 同构（模板逐字复用，五域先例）。
func (d *DictTypePO) BeforeCreate(tx *gorm.DB) error {
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

// DictDataPO sys_dict_data 表映射（13 列，列名对齐真实 DDL，snake_case）。
// dict_type_name 为服务端冗余列——add/edit 校验类型存在后由 service 写入类型的
// dict_name（源语义，不信请求体同名字段）。dict_type_id 业务上
// 必有值（create 前置类型存在性校验），故非指针。
type DictDataPO struct {
	BaseWithOperator

	ID           int64   `gorm:"column:id;primaryKey"`
	DictTypeID   int64   `gorm:"column:dict_type_id"`
	DictTypeName *string `gorm:"column:dict_type_name"`
	Label        *string `gorm:"column:label"`
	Value        *string `gorm:"column:value"`
	Sort         *int    `gorm:"column:sort"`
}

// TableName 显式表名（Tabler 接口）。
func (DictDataPO) TableName() string {
	return "sys_dict_data"
}

// BeforeCreate 生成雪花 ID + 双时间 + ctx identity 四操作人填充（同构模板）。
func (d *DictDataPO) BeforeCreate(tx *gorm.DB) error {
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
