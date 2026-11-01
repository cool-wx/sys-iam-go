package po

import (
	"time"

	"gorm.io/gorm"

	"github.com/cool-wx/sys-iam-go/internal/auth"
	"github.com/cool-wx/sys-iam-go/pkg/ids"
)

// SysConfigPO sys_config 表映射（11 列，列名对齐真实 DDL，snake_case）。
// config_key 列由 DB 层 UNIQUE KEY uk_config_key 约束（CONF-01 判重 1062 语义）。
type SysConfigPO struct {
	BaseWithOperator

	ID          int64  `gorm:"column:id;primaryKey"`
	ConfigKey   string `gorm:"column:config_key"`
	ConfigValue string `gorm:"column:config_value"`
	ConfigType  string `gorm:"column:config_type"`
}

// TableName 显式表名（Tabler 接口）。
func (SysConfigPO) TableName() string {
	return "sys_config"
}

// BeforeCreate 生成雪花 ID + 双时间 + ctx identity 四操作人填充（同构模板）。
func (c *SysConfigPO) BeforeCreate(tx *gorm.DB) error {
	if c.ID == 0 {
		c.ID = int64(ids.NewID())
	}
	now := time.Now()
	c.CreateTime = &now
	c.UpdateTime = &now
	if id, ok := auth.FromContext(tx.Statement.Context); ok {
		c.CreateByID = &id.UserID
		c.CreateByName = &id.Name
		c.UpdateByID = &id.UserID
		c.UpdateByName = &id.Name
	}
	return nil
}
