// sys_file 数据访问（照 repository/menu.go 模板；List 为分页闭包——源 /file list
// 是分页 1/15 + categoryId 可选过滤 + create_time 倒序；无 dataScope）。
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

// FileInfoRepo sys_file 数据访问。
type FileInfoRepo struct {
	db *gorm.DB
}

// NewFileInfoRepo 构造（db 可为 *gorm.DB 或事务 tx——上传链在事务内以 tx 重建本 repo）。
func NewFileInfoRepo(db *gorm.DB) *FileInfoRepo {
	return &FileInfoRepo{db: db}
}

// Create 插入一行；ctx 经 WithContext 传递，供审计钩子读取 identity。
// 参数化由 GORM 占位符保证，不拼 SQL 字符串。
func (r *FileInfoRepo) Create(ctx context.Context, f *po.FileInfoPO) error {
	return r.db.WithContext(ctx).Create(f).Error
}

// Patch 显式列更新（照 MenuRepo.Patch 模板：白名单 map + 恒带 Where("id = ?") +
// RowsAffected==0 时 Count 二次确认 + 显式并入审计列——GORM 对 Updates(map)
// 不写钩子改动）。
func (r *FileInfoRepo) Patch(ctx context.Context, id int64, cols map[string]any) error {
	if cols == nil {
		cols = map[string]any{}
	}
	now := time.Now()
	cols["update_time"] = now
	if identity, ok := auth.FromContext(ctx); ok {
		cols["update_by_id"] = identity.UserID
		cols["update_by_name"] = identity.Name
	}
	res := r.db.WithContext(ctx).Model(&po.FileInfoPO{}).Where("id = ?", id).Updates(cols)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		var n int64
		if err := r.db.WithContext(ctx).Model(&po.FileInfoPO{}).Where("id = ?", id).Count(&n).Error; err != nil {
			return err
		}
		if n == 0 {
			return errcode.ErrNotFound
		}
	}
	return nil
}

// Delete 物理删除（硬删除语义）。不校验存在性：删除不存在的 id 静默成功
// （源 removeById 语义）；存储侧对象删除由 service 层先行处理（宽容语义）。
func (r *FileInfoRepo) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&po.FileInfoPO{}).Error
}

// FindByID 按主键查询；未命中返回 ErrNotFound（10300）。
func (r *FileInfoRepo) FindByID(ctx context.Context, id int64) (*po.FileInfoPO, error) {
	var f po.FileInfoPO
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&f).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errcode.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &f, nil
}

// FindByCategoryAndHash 上传判重查询（源 FileInfoRepositoryImpl:29-37
// 蓝本逐字语义）：categoryId 非 nil → WHERE category_id = ?，nil → WHERE
// category_id IS NULL——双分支缺一不可：唯一索引对 NULL 视为互不相等，未分类
// 上传的判重全靠 IS NULL 分支（否则未分类可无限重复上传同内容）。恒带
// AND file_hash = ? + LIMIT 1。未命中返回 (nil, nil)——"无重复"非错误。
func (r *FileInfoRepo) FindByCategoryAndHash(ctx context.Context, categoryID *int64, fileHash string) (*po.FileInfoPO, error) {
	q := r.db.WithContext(ctx).Model(&po.FileInfoPO{})
	if categoryID != nil {
		q = q.Where("category_id = ?", *categoryID)
	} else {
		q = q.Where("category_id IS NULL")
	}
	var f po.FileInfoPO
	err := q.Where("file_hash = ?", fileHash).Limit(1).First(&f).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &f, nil
}

// List 分页列表（源 /file list 语义直译）：page/size 已由 handler 层 paging.Parse
// 钳位；categoryID 非 nil 时可选过滤；ORDER BY create_time DESC, id DESC（源排序，
// id ASC tiebreaker 保证同秒稳定序）。无 dataScope（本阶段零应用）。
func (r *FileInfoRepo) List(ctx context.Context, page, size int, categoryID *int64) ([]po.FileInfoPO, int64, error) {
	q := func() *gorm.DB {
		query := r.db.WithContext(ctx).Model(&po.FileInfoPO{})
		if categoryID != nil {
			query = query.Where("category_id = ?", *categoryID)
		}
		return query
	}
	var total int64
	if err := q().Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * size
	if offset < 0 {
		// int 溢出保护（双保险，照 UserRepo.List 同构）
		offset = 0
	}
	var files []po.FileInfoPO
	err := q().Offset(offset).Limit(size).
		Order("create_time DESC, id DESC").
		Find(&files).Error
	if err != nil {
		return nil, 0, err
	}
	return files, total, nil
}
