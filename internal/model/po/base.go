// Package po 定义 GORM 持久化对象：22 列原生类型 + 显式 column tag，对齐 sys_user 真实 DDL。
package po

import "time"

// BaseWithOperator 审计列内嵌（等价源 BaseAggregate + BaseUserEntity）。
// 全部指针原生类型——无自定义类型进 PO（P1 根治 Valuer/Scanner 陷阱）；
// 无 DeletedAt（硬删除语义，P4）。
type BaseWithOperator struct {
	CreateByID   *int64     `gorm:"column:create_by_id"`
	CreateByName *string    `gorm:"column:create_by_name"`
	UpdateByID   *int64     `gorm:"column:update_by_id"`
	UpdateByName *string    `gorm:"column:update_by_name"`
	CreateTime   *time.Time `gorm:"column:create_time"`
	UpdateTime   *time.Time `gorm:"column:update_time"`
	Remark       *string    `gorm:"column:remark"`
}
