// sys_menu 数据访问（List 无分页全量——树查询需要完整数据集）。
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

// MenuRepo sys_menu 数据访问。
type MenuRepo struct {
	db *gorm.DB
}

// NewMenuRepo 构造（db 可为 *gorm.DB 或事务 tx）。
func NewMenuRepo(db *gorm.DB) *MenuRepo {
	return &MenuRepo{db: db}
}

// Create 插入一行；ctx 经 WithContext 传递，供审计钩子读取 identity。
// 参数化由 GORM 占位符保证，不拼 SQL 字符串。
func (r *MenuRepo) Create(ctx context.Context, m *po.MenuPO) error {
	return r.db.WithContext(ctx).Create(m).Error
}

// Patch 显式列更新（零值保护核心；回读确认语义与 UserRepo.Patch
// 一致）：只写 cols 中出现的列（白名单由调用方 service 层收敛）；恒带
// Where("id = ?") 防全表更新。update_time 与审计操作人列在此显式并入
// map——GORM 对 Updates(map) 不写钩子改动的结构体字段（已知语义坑），显式并入
// 保证确定性。绝不在更新路径调用 Save。
func (r *MenuRepo) Patch(ctx context.Context, id int64, cols map[string]any) error {
	if cols == nil {
		cols = map[string]any{}
	}
	now := time.Now()
	cols["update_time"] = now
	if identity, ok := auth.FromContext(ctx); ok {
		cols["update_by_id"] = identity.UserID
		cols["update_by_name"] = identity.Name
	}
	res := r.db.WithContext(ctx).Model(&po.MenuPO{}).Where("id = ?", id).Updates(cols)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		// 0 变更 ≠ 不存在：同秒重复提交相同内容时 SET 值全等 → 0 行变更但行存在，
		// 需 Count 二次确认，仅当行确实不存在才返回 ErrNotFound
		var n int64
		if err := r.db.WithContext(ctx).Model(&po.MenuPO{}).Where("id = ?", id).Count(&n).Error; err != nil {
			return err
		}
		if n == 0 {
			return errcode.ErrNotFound
		}
	}
	return nil
}

// Delete 物理删除（硬删除语义）。不校验子菜单/角色引用，直接删除——删除带子节点的
// 菜单仍成功，孤儿子节点经 tree.Build 孤儿提升语义在树查询中可见。
// 不校验存在性：删除不存在的 id 静默成功。
func (r *MenuRepo) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&po.MenuPO{}).Error
}

// List 全量查询，ORDER BY sort ASC, id ASC（id ASC tiebreaker 保证同 sort 稳定序）。
// 无分页：树查询需要完整数据集（前端一次性拉全量建树）。
func (r *MenuRepo) List(ctx context.Context) ([]po.MenuPO, error) {
	var menus []po.MenuPO
	err := r.db.WithContext(ctx).Model(&po.MenuPO{}).
		Order("sort ASC, id ASC").
		Find(&menus).Error
	if err != nil {
		return nil, err
	}
	return menus, nil
}

// FindByID 按主键查询；未命中返回 ErrNotFound（10300）。
func (r *MenuRepo) FindByID(ctx context.Context, id int64) (*po.MenuPO, error) {
	var m po.MenuPO
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errcode.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// FindByIDs 批量按主键查询（WHERE id IN ?）；供角色 assignMenus 前置全量校验
// 与冗余 menuName 装配使用。缺失的 id 自然缺席结果集，
// 由调用方对比求缺失集合。
func (r *MenuRepo) FindByIDs(ctx context.Context, ids []int64) ([]po.MenuPO, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var menus []po.MenuPO
	err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&menus).Error
	if err != nil {
		return nil, err
	}
	return menus, nil
}
