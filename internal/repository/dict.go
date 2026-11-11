// sys_dict_type/sys_dict_data 数据访问（照 repository/menu.go + user.go List 模板；
// List 分页闭包无 dataScope——本阶段零 dataScope 应用，闭包内不得出现
// filter.Apply）。
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

// DictTypeRepo sys_dict_type 数据访问。
type DictTypeRepo struct {
	db *gorm.DB
}

// NewDictTypeRepo 构造（db 可为 *gorm.DB 或事务 tx）。
func NewDictTypeRepo(db *gorm.DB) *DictTypeRepo {
	return &DictTypeRepo{db: db}
}

// Create 插入一行；ctx 经 WithContext 传递，供审计钩子读取 identity。
// uk_dict_type 冲突时（TranslateError 开启）返回 gorm.ErrDuplicatedKey，
// 由 service 层翻译为 400/10400「字典类型已存在」。
// 参数化由 GORM 占位符保证，不拼 SQL 字符串。
func (r *DictTypeRepo) Create(ctx context.Context, m *po.DictTypePO) error {
	return r.db.WithContext(ctx).Create(m).Error
}

// Patch 显式列更新（零值保护核心；回读确认语义与 MenuRepo.Patch
// 一致）：只写 cols 中出现的列（白名单由调用方 service 层收敛）；恒带
// Where("id = ?") 防全表更新。update_time 与审计操作人列在此显式并入
// map——GORM 对 Updates(map) 不写钩子改动的结构体字段（已知语义坑），显式并入
// 保证确定性。绝不在更新路径调用 Save。
func (r *DictTypeRepo) Patch(ctx context.Context, id int64, cols map[string]any) error {
	if cols == nil {
		cols = map[string]any{}
	}
	now := time.Now()
	cols["update_time"] = now
	if identity, ok := auth.FromContext(ctx); ok {
		cols["update_by_id"] = identity.UserID
		cols["update_by_name"] = identity.Name
	}
	res := r.db.WithContext(ctx).Model(&po.DictTypePO{}).Where("id = ?", id).Updates(cols)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		// 0 变更 ≠ 不存在：同秒重复提交相同内容时 SET 值全等 → 0 行变更但行存在，
		// 需 Count 二次确认，仅当行确实不存在才返回 ErrNotFound
		var n int64
		if err := r.db.WithContext(ctx).Model(&po.DictTypePO{}).Where("id = ?", id).Count(&n).Error; err != nil {
			return err
		}
		if n == 0 {
			return errcode.ErrNotFound
		}
	}
	return nil
}

// Delete 物理删除（硬删除语义）。不校验存在性：删除不存在的 id 静默成功
// （源 deleteDictType 语义）。与源一致**不加字典数据引用检查**——删除类型后
// 其下 dict_data 成为孤儿（源缺口按源保留）。
func (r *DictTypeRepo) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&po.DictTypePO{}).Error
}

// List 分页查询（Count 与查询同一闭包防 total 漂移——user.go List 先例）；
// 无 dataScope（闭包内无 filter.Apply）；ORDER BY create_time DESC, id DESC
// （源 PageHelper orderByDesc(createTime) + id tiebreaker 稳定序）。
func (r *DictTypeRepo) List(ctx context.Context, page, size int) ([]po.DictTypePO, int64, error) {
	q := func() *gorm.DB {
		return r.db.WithContext(ctx).Model(&po.DictTypePO{})
	}
	var total int64
	if err := q().Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * size
	if offset < 0 {
		offset = 0
	}
	var items []po.DictTypePO
	err := q().Offset(offset).Limit(size).
		Order("create_time DESC, id DESC").
		Find(&items).Error
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// FindByID 按主键查询；未命中返回 ErrNotFound（10300）。供 dict_data add/edit
// 的类型存在性校验复用（FindByTypeAndID 语义）。
func (r *DictTypeRepo) FindByID(ctx context.Context, id int64) (*po.DictTypePO, error) {
	var m po.DictTypePO
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errcode.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// DictDataRepo sys_dict_data 数据访问。
type DictDataRepo struct {
	db *gorm.DB
}

// NewDictDataRepo 构造（db 可为 *gorm.DB 或事务 tx）。
func NewDictDataRepo(db *gorm.DB) *DictDataRepo {
	return &DictDataRepo{db: db}
}

// Create 插入一行；ctx 经 WithContext 传递，供审计钩子读取 identity。
// sys_dict_data 无唯一索引——不触发 1062。
func (r *DictDataRepo) Create(ctx context.Context, m *po.DictDataPO) error {
	return r.db.WithContext(ctx).Create(m).Error
}

// Patch 显式列更新（零值保护核心；语义与 DictTypeRepo.Patch 一致）。
func (r *DictDataRepo) Patch(ctx context.Context, id int64, cols map[string]any) error {
	if cols == nil {
		cols = map[string]any{}
	}
	now := time.Now()
	cols["update_time"] = now
	if identity, ok := auth.FromContext(ctx); ok {
		cols["update_by_id"] = identity.UserID
		cols["update_by_name"] = identity.Name
	}
	res := r.db.WithContext(ctx).Model(&po.DictDataPO{}).Where("id = ?", id).Updates(cols)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		var n int64
		if err := r.db.WithContext(ctx).Model(&po.DictDataPO{}).Where("id = ?", id).Count(&n).Error; err != nil {
			return err
		}
		if n == 0 {
			return errcode.ErrNotFound
		}
	}
	return nil
}

// Delete 物理删除（硬删除语义）。不校验存在性：删除不存在的 id 静默成功
// （源 deleteDictData 语义）。
func (r *DictDataRepo) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&po.DictDataPO{}).Error
}

// List 分页查询；dictTypeID 非零时 Where dict_type_id = ?（可选过滤分支，
// 源 list?dictTypeId= 语义——不传返回全量分页）。
func (r *DictDataRepo) List(ctx context.Context, page, size int, dictTypeID int64) ([]po.DictDataPO, int64, error) {
	q := func() *gorm.DB {
		query := r.db.WithContext(ctx).Model(&po.DictDataPO{})
		if dictTypeID != 0 {
			query = query.Where("dict_type_id = ?", dictTypeID)
		}
		return query
	}
	var total int64
	if err := q().Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * size
	if offset < 0 {
		offset = 0
	}
	var items []po.DictDataPO
	err := q().Offset(offset).Limit(size).
		Order("create_time DESC, id DESC").
		Find(&items).Error
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// FindByID 按主键查询；未命中返回 ErrNotFound（10300）。
func (r *DictDataRepo) FindByID(ctx context.Context, id int64) (*po.DictDataPO, error) {
	var m po.DictDataPO
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errcode.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}
