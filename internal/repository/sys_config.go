// sys_config 数据访问（照 repository/dict.go 模板；FindByKey 供 CONF-01 key 读取）。
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

// SysConfigRepo sys_config 数据访问。
type SysConfigRepo struct {
	db *gorm.DB
}

// NewSysConfigRepo 构造（db 可为 *gorm.DB 或事务 tx）。
func NewSysConfigRepo(db *gorm.DB) *SysConfigRepo {
	return &SysConfigRepo{db: db}
}

// Create 插入一行；uk_config_key 冲突时（TranslateError 开启）返回
// gorm.ErrDuplicatedKey，由 service 层翻译为 400/10400「配置键已存在」（CONF-01）。
// 参数化由 GORM 占位符保证，不拼 SQL 字符串（T-1-03）。
func (r *SysConfigRepo) Create(ctx context.Context, m *po.SysConfigPO) error {
	return r.db.WithContext(ctx).Create(m).Error
}

// Patch 显式列更新（零值保护核心，ENG-05；CR-01 回读确认语义同族）。
func (r *SysConfigRepo) Patch(ctx context.Context, id int64, cols map[string]any) error {
	if cols == nil {
		cols = map[string]any{}
	}
	now := time.Now()
	cols["update_time"] = now
	if identity, ok := auth.FromContext(ctx); ok {
		cols["update_by_id"] = identity.UserID
		cols["update_by_name"] = identity.Name
	}
	res := r.db.WithContext(ctx).Model(&po.SysConfigPO{}).Where("id = ?", id).Updates(cols)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		// 0 变更 ≠ 不存在：同秒重复提交相同内容时 SET 值全等 → 0 行变更但行存在，
		// 需 Count 二次确认（CR-01）
		var n int64
		if err := r.db.WithContext(ctx).Model(&po.SysConfigPO{}).Where("id = ?", id).Count(&n).Error; err != nil {
			return err
		}
		if n == 0 {
			return errcode.ErrNotFound
		}
	}
	return nil
}

// Delete 物理删除（硬删除语义，P4）。不校验存在性：删除不存在的 id 静默成功
// （源 deleteConfig 语义）。
func (r *SysConfigRepo) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&po.SysConfigPO{}).Error
}

// List 分页查询（Count 与查询同一闭包防 total 漂移）；无条件过滤（无 dataScope
// ——D-03 拍板零应用）；ORDER BY create_time DESC, id DESC（源列序惯例）。
func (r *SysConfigRepo) List(ctx context.Context, page, size int) ([]po.SysConfigPO, int64, error) {
	q := func() *gorm.DB {
		return r.db.WithContext(ctx).Model(&po.SysConfigPO{})
	}
	var total int64
	if err := q().Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * size
	if offset < 0 {
		offset = 0
	}
	var items []po.SysConfigPO
	err := q().Offset(offset).Limit(size).
		Order("create_time DESC, id DESC").
		Find(&items).Error
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// FindByID 按主键查询；未命中返回 ErrNotFound（10300）。
func (r *SysConfigRepo) FindByID(ctx context.Context, id int64) (*po.SysConfigPO, error) {
	var m po.SysConfigPO
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errcode.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// FindByKey 按 config_key 精确查询（uk_config_key 保证至多一行）；未命中返回
// ErrNotFound（10300）——CONF-01 getByKey 语义（源返回 configValue 字符串，
// 不存在由 service 层翻译「配置不存在: {key}」）。
func (r *SysConfigRepo) FindByKey(ctx context.Context, key string) (*po.SysConfigPO, error) {
	var m po.SysConfigPO
	err := r.db.WithContext(ctx).Where("config_key = ?", key).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errcode.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}
