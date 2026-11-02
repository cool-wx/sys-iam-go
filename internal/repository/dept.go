// sys_dept 数据访问（与 repository/menu.go 同构；List 无分页全量——树与源 list 语义）。
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

// DeptRepo sys_dept 数据访问。
type DeptRepo struct {
	db *gorm.DB
}

// NewDeptRepo 构造（db 可为 *gorm.DB 或事务 tx）。
func NewDeptRepo(db *gorm.DB) *DeptRepo {
	return &DeptRepo{db: db}
}

// Create 插入一行；ctx 经 WithContext 传递，供审计钩子读取 identity。
func (r *DeptRepo) Create(ctx context.Context, d *po.DeptPO) error {
	return r.db.WithContext(ctx).Create(d).Error
}

// Patch 显式列更新（零值保护核心；回读确认语义与 UserRepo.Patch
// 一致）：只写 cols 中出现的列（白名单由调用方 service 层收敛）；恒带
// Where("id = ?") 防全表更新。update_time 与审计操作人列在此显式并入
// map（GORM Updates(map) 钩子语义坑），绝不在更新路径调用 Save。
func (r *DeptRepo) Patch(ctx context.Context, id int64, cols map[string]any) error {
	if cols == nil {
		cols = map[string]any{}
	}
	now := time.Now()
	cols["update_time"] = now
	if identity, ok := auth.FromContext(ctx); ok {
		cols["update_by_id"] = identity.UserID
		cols["update_by_name"] = identity.Name
	}
	res := r.db.WithContext(ctx).Model(&po.DeptPO{}).Where("id = ?", id).Updates(cols)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		// 0 变更 ≠ 不存在：Count 二次确认
		var n int64
		if err := r.db.WithContext(ctx).Model(&po.DeptPO{}).Where("id = ?", id).Count(&n).Error; err != nil {
			return err
		}
		if n == 0 {
			return errcode.ErrNotFound
		}
	}
	return nil
}

// Delete 物理删除（硬删除语义，P4）。与源一致**不加子部门/用户引用检查**
// （源缺口按源保留：DeptApplicationService.removeById 直接删）。不校验存在性。
func (r *DeptRepo) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&po.DeptPO{}).Error
}

// List 全量查询，ORDER BY sort ASC, id ASC（源 DeptRepositoryImpl list
// orderByAsc(sort)；id ASC tiebreaker 保证稳定序）。无分页：树查询与源 list 语义。
func (r *DeptRepo) List(ctx context.Context) ([]po.DeptPO, error) {
	var depts []po.DeptPO
	err := r.db.WithContext(ctx).Model(&po.DeptPO{}).
		Order("sort ASC, id ASC").
		Find(&depts).Error
	if err != nil {
		return nil, err
	}
	return depts, nil
}

// FindByID 按主键查询；未命中返回 ErrNotFound（10300）。
func (r *DeptRepo) FindByID(ctx context.Context, id int64) (*po.DeptPO, error) {
	var d po.DeptPO
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&d).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errcode.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// FindByIDs 批量按主键查询（WHERE id IN ?，照 MenuRepo.FindByIDs 模板）；供
// 角色数据权限范围分配（AssignScopes）前置全量校验与 dept_name 冗余装配
// 使用。缺失的 id 自然缺席结果集，由调用方对比求缺失集合。
func (r *DeptRepo) FindByIDs(ctx context.Context, ids []int64) ([]po.DeptPO, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var depts []po.DeptPO
	err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&depts).Error
	if err != nil {
		return nil, err
	}
	return depts, nil
}
