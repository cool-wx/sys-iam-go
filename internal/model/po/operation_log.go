package po

import (
	"time"

	"gorm.io/gorm"

	"github.com/cool-wx/sys-iam-go/pkg/ids"
)

// OperationLogPO sys_operation_log 表映射（14 列，列名对齐真实 DDL
// scripts/ddl/sys_operation_log.sql，snake_case）。
//
// 刻意**不内嵌 BaseWithOperator**（DDL 实证）：本表无 create_by_id/create_by_name/
// update_by_id/update_by_name 四操作人列——仅内嵌裁剪版 base（create_time/update_time/remark 三列）。
type OperationLogPO struct {
	// base 裁剪版审计列（无操作人四列——DDL 实证 sys_operation_log 不含）。
	CreateTime *time.Time `gorm:"column:create_time"`
	UpdateTime *time.Time `gorm:"column:update_time"`
	Remark     *string    `gorm:"column:remark"`

	ID       int64   `gorm:"column:id;primaryKey"`
	UserID   *int64  `gorm:"column:user_id"` // 未登录（如 login 路由自身）为 NULL
	RealName *string `gorm:"column:real_name;size:50"`
	Module   *string `gorm:"column:module;size:100"`
	// Operation 路由声明的方法名（声明制，如 "create"/"login"）。
	Operation *string `gorm:"column:operation;size:200"`
	// Method 形如 "POST /api/v1/dict-types"（HTTP 方法 + 请求 URI 拼接）。
	Method *string `gorm:"column:method;size:200"`
	// Params 请求体 JSON 回放（8 敏感字段已掩码 + 按字节截 60000——Oplog 中间件组装）。
	Params *string `gorm:"column:params;type:text"`
	IP     *string `gorm:"column:ip;size:50"`
	// Duration 请求耗时毫秒。
	Duration *int `gorm:"column:duration"`
	// Status 成功 1 / 失败 0（HTTP <400 → 1，否则 0；DDL default NULL 由应用恒写）。
	Status *int8 `gorm:"column:status;size:1"`
	// ErrorMsg 服务端错误消息（resp.Fail 经 c.Error 透出，rune 截 500）。
	ErrorMsg *string `gorm:"column:error_msg;type:text"`
}

// TableName 显式表名（Tabler 接口）。
func (OperationLogPO) TableName() string {
	return "sys_operation_log"
}

// BeforeCreate 雪花 ID + 双时间（**无操作人填充**——本表无四操作人列）。
// 操作人由 Oplog 中间件直接写入 UserID/RealName 业务列
// （审计语义不同于业务表的四操作人列，不走 ctx 钩子填充）。
func (l *OperationLogPO) BeforeCreate(tx *gorm.DB) error {
	if l.ID == 0 {
		l.ID = int64(ids.NewID())
	}
	now := time.Now()
	l.CreateTime = &now
	l.UpdateTime = &now
	return nil
}
