package po

import (
	"time"

	"gorm.io/gorm"

	"github.com/cool-wx/sys-iam-go/internal/auth"
	"github.com/cool-wx/sys-iam-go/pkg/ids"
)

// ApiKeyPO sys_api_key 表映射（13 列，列名对齐真实 DDL，snake_case）。
// ApiKey 明文存储为源语义（schema 沿用禁改，T-2-06 accept）；明文仅在 add 响应一次性返回。
type ApiKeyPO struct {
	BaseWithOperator

	ID       int64      `gorm:"column:id;primaryKey"`
	ApiKey   string     `gorm:"column:api_key"`
	Username string     `gorm:"column:username"`
	UserID   *int64     `gorm:"column:user_id"`
	// ExpireTime 过期时间（NULL=永不过期；登录过滤语义 expire_time > NOW()）。
	ExpireTime *time.Time `gorm:"column:expire_time"`
	Status     *int8      `gorm:"column:status"`
}

// TableName 显式表名（Tabler 接口）。
func (ApiKeyPO) TableName() string {
	return "sys_api_key"
}

// BeforeCreate 生成雪花 ID（若未预置）+ 填充双时间 + 从 ctx identity 填充四操作人列。
func (k *ApiKeyPO) BeforeCreate(tx *gorm.DB) error {
	if k.ID == 0 {
		k.ID = int64(ids.NewID())
	}
	now := time.Now()
	k.CreateTime = &now
	k.UpdateTime = &now
	if id, ok := auth.FromContext(tx.Statement.Context); ok {
		k.CreateByID = &id.UserID
		k.CreateByName = &id.Name
		k.UpdateByID = &id.UserID
		k.UpdateByName = &id.Name
	}
	return nil
}
