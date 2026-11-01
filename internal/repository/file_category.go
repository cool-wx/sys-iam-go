// sys_file_category 数据访问（与 repository/dept.go 同构；List 无分页全量——树
// 与源 /fileCategory list 语义）。
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

// FileCategoryRepo sys_file_category 数据访问。
type FileCategoryRepo struct {
	db *gorm.DB
}

// NewFileCategoryRepo 构造（db 可为 *gorm.DB 或事务 tx）。
func NewFileCategoryRepo(db *gorm.DB) *FileCategoryRepo {
	return &FileCategoryRepo{db: db}
}

// Create 插入一行；ctx 经 WithContext 传递，供审计钩子读取 identity。
func (r *FileCategoryRepo) Create(ctx context.Context, fc *po.FileCategoryPO) error {
	return r.db.WithContext(ctx).Create(fc).Error
}

// Patch 显式列更新（照 MenuRepo.Patch 模板：白名单 map + 恒带 Where("id = ?") +
// RowsAffected==0 时 Count 二次确认 CR-01 + 显式并入审计列）。
func (r *FileCategoryRepo) Patch(ctx context.Context, id int64, cols map[string]any) error {
	if cols == nil {
		cols = map[string]any{}
	}
	now := time.Now()
	cols["update_time"] = now
	if identity, ok := auth.FromContext(ctx); ok {
		cols["update_by_id"] = identity.UserID
		cols["update_by_name"] = identity.Name
	}
	res := r.db.WithContext(ctx).Model(&po.FileCategoryPO{}).Where("id = ?", id).Updates(cols)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		var n int64
		if err := r.db.WithContext(ctx).Model(&po.FileCategoryPO{}).Where("id = ?", id).Count(&n).Error; err != nil {
			return err
		}
		if n == 0 {
			return errcode.ErrNotFound
		}
	}
	return nil
}

// Delete 物理删除（硬删除语义，P4）。与源一致**不加子分类/文件引用检查**
// （源缺口按源保留：FileCategoryApplicationService.removeById 直接删——删除带
// 子节点的分类仍成功，孤儿子分类经 tree.Build 孤儿提升语义在树查询中可见）。
// 不校验存在性：删除不存在的 id 静默成功（源 removeById 语义）。
func (r *FileCategoryRepo) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&po.FileCategoryPO{}).Error
}

// List 全量查询，ORDER BY sort ASC, id ASC（源 FileCategoryRepositoryImpl list
// orderByAsc(sort)；id ASC tiebreaker 保证稳定序）。无分页：树查询与源 list 语义。
func (r *FileCategoryRepo) List(ctx context.Context) ([]po.FileCategoryPO, error) {
	var cats []po.FileCategoryPO
	err := r.db.WithContext(ctx).Model(&po.FileCategoryPO{}).
		Order("sort ASC, id ASC").
		Find(&cats).Error
	if err != nil {
		return nil, err
	}
	return cats, nil
}

// FindByID 按主键查询；未命中返回 ErrNotFound（10300）。
func (r *FileCategoryRepo) FindByID(ctx context.Context, id int64) (*po.FileCategoryPO, error) {
	var fc po.FileCategoryPO
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&fc).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errcode.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &fc, nil
}
