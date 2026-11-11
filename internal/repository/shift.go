// sys_shift 数据访问（List 全量无分页——此接口不分页、返回全量列表，
// 勿统一为分页）。
package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/cool-wx/sys-iam-go/internal/auth"
	"github.com/cool-wx/sys-iam-go/internal/model/po"
	"github.com/cool-wx/sys-iam-go/pkg/errcode"
)

// ShiftRepo sys_shift 数据访问。
type ShiftRepo struct {
	db *gorm.DB
}

// NewShiftRepo 构造（db 可为 *gorm.DB 或事务 tx）。
func NewShiftRepo(db *gorm.DB) *ShiftRepo {
	return &ShiftRepo{db: db}
}

// Create 插入一行；sys_shift 无唯一索引——不触发 1062。
func (r *ShiftRepo) Create(ctx context.Context, m *po.ShiftPO) error {
	return r.db.WithContext(ctx).Create(m).Error
}

// Patch 显式列更新（零值保护核心；回读确认语义同族）。
func (r *ShiftRepo) Patch(ctx context.Context, id int64, cols map[string]any) error {
	if cols == nil {
		cols = map[string]any{}
	}
	now := time.Now()
	cols["update_time"] = now
	if identity, ok := auth.FromContext(ctx); ok {
		cols["update_by_id"] = identity.UserID
		cols["update_by_name"] = identity.Name
	}
	res := r.db.WithContext(ctx).Model(&po.ShiftPO{}).Where("id = ?", id).Updates(cols)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		var n int64
		if err := r.db.WithContext(ctx).Model(&po.ShiftPO{}).Where("id = ?", id).Count(&n).Error; err != nil {
			return err
		}
		if n == 0 {
			return errcode.ErrNotFound
		}
	}
	return nil
}

// Delete 物理删除（硬删除语义）。不校验存在性：删除不存在的 id 静默成功。
func (r *ShiftRepo) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&po.ShiftPO{}).Error
}

// List 全量查询（无分页——返回全量列表，勿统一为分页），
// ORDER BY shift_code ASC, id ASC（id tiebreaker 稳定序）。
func (r *ShiftRepo) List(ctx context.Context) ([]po.ShiftPO, error) {
	var items []po.ShiftPO
	err := r.db.WithContext(ctx).Model(&po.ShiftPO{}).
		Order("shift_code ASC, id ASC").
		Find(&items).Error
	if err != nil {
		return nil, err
	}
	return items, nil
}

// FindByID 按主键查询；未命中返回 ErrNotFound（10300）。
func (r *ShiftRepo) FindByID(ctx context.Context, id int64) (*po.ShiftPO, error) {
	var m po.ShiftPO
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errcode.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}
