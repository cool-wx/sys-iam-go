package repository

import (
	"context"

	"gorm.io/gorm"

	"github.com/cool-wx/sys-iam-go/internal/model/po"
)

// OlogListFilter 操作日志列表过滤条件（三 like 模糊——源 LogQuery 字段语义：
// module/operation/realName 全 like；空串=不过滤）。
type OlogListFilter struct {
	Module    string
	Operation string
	RealName  string
}

// OperationLogRepo sys_operation_log 数据访问（写侧供 OpLogWriter 批量落库，
// 读侧供日志查询域分页检索）。
type OperationLogRepo struct {
	db *gorm.DB
}

// NewOperationLogRepo 构造。
func NewOperationLogRepo(db *gorm.DB) *OperationLogRepo {
	return &OperationLogRepo{db: db}
}

// InsertBatch 批量插入（OpLogWriter worker 消费——批量落库失败由 writer 层
// slog.Warn 静默丢弃，绝不反压业务）。参数化由 GORM 占位符保证（T-1-03）。
func (r *OperationLogRepo) InsertBatch(ctx context.Context, entries []po.OperationLogPO) error {
	if len(entries) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).CreateInBatches(&entries, len(entries)).Error
}

// List 分页查询：Count 与记录查询经同一闭包构造（条件集天然一致，防 total 漂移，
// P15）。无 dataScope（D-03 本阶段零应用——闭包结构照 user.go List 去掉 filter.Apply）。
// 三条件 like 模糊（值经占位符参数化，% 拼接在参数侧不进 SQL 文本——T-1-03）；
// 排序 create_time DESC, id DESC（源 orderByDesc createTime + 稳定 tiebreaker）；
// 排序串为代码常量，无 ORDER 注入面（T-1-10）。
// page/size 已由 paging.ParseWithDefaults 钳位（≥1 且 page ≤ maxPage）；Offset
// 计算后仍防负（WR-03 双保险，P8）。
func (r *OperationLogRepo) List(ctx context.Context, page, size int, f OlogListFilter) ([]po.OperationLogPO, int64, error) {
	q := func() *gorm.DB {
		query := r.db.WithContext(ctx).Model(&po.OperationLogPO{})
		if f.Module != "" {
			query = query.Where("module LIKE ?", "%"+f.Module+"%")
		}
		if f.Operation != "" {
			query = query.Where("operation LIKE ?", "%"+f.Operation+"%")
		}
		if f.RealName != "" {
			query = query.Where("real_name LIKE ?", "%"+f.RealName+"%")
		}
		return query
	}
	var total int64
	if err := q().Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * size
	if offset < 0 {
		// int 溢出保护（WR-03，双保险）：GORM clause.Limit 仅在 Offset>0 时写
		// OFFSET 子句，溢出为负会被静默省略 → 静默回退第 1 页数据。
		offset = 0
	}
	var logs []po.OperationLogPO
	err := q().Offset(offset).Limit(size).
		Order("create_time DESC, id DESC").
		Find(&logs).Error
	if err != nil {
		return nil, 0, err
	}
	return logs, total, nil
}
