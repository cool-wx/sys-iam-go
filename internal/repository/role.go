// sys_role 数据访问（照 repository/user.go 模板；List 为全库唯一 ORDER BY id ASC
// 特例——源 RoleRepositoryImpl.selectPage orderByAsc(id)，勿套用其他域的
// create_time DESC 模板，02-RESEARCH 1.3 源特例实证）。
package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/cool-wx/sys-iam-go/internal/auth"
	"github.com/cool-wx/sys-iam-go/internal/model/po"
	"github.com/cool-wx/sys-iam-go/pkg/errcode"
)

// RoleRepo sys_role 数据访问。
type RoleRepo struct {
	db *gorm.DB
}

// NewRoleRepo 构造（db 可为 *gorm.DB 或事务 tx）。
func NewRoleRepo(db *gorm.DB) *RoleRepo {
	return &RoleRepo{db: db}
}

// Create 插入一行；ctx 经 WithContext 传递，供审计钩子读取 identity。
func (r *RoleRepo) Create(ctx context.Context, role *po.RolePO) error {
	return r.db.WithContext(ctx).Create(role).Error
}

// Patch 显式列更新（零值保护核心，ENG-05；CR-01 回读确认语义与 UserRepo.Patch
// 一致）：只写 cols 中出现的列（白名单由调用方 service 层收敛，T-1-10）；恒带
// Where("id = ?") 防全表更新（T-1-08）。update_time 与审计操作人列在此显式并入
// map（GORM Updates(map) 钩子语义坑），绝不在更新路径调用 Save（Pitfall 3）。
func (r *RoleRepo) Patch(ctx context.Context, id int64, cols map[string]any) error {
	if cols == nil {
		cols = map[string]any{}
	}
	now := time.Now()
	cols["update_time"] = now
	if identity, ok := auth.FromContext(ctx); ok {
		cols["update_by_id"] = identity.UserID
		cols["update_by_name"] = identity.Name
	}
	res := r.db.WithContext(ctx).Model(&po.RolePO{}).Where("id = ?", id).Updates(cols)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		// 0 变更 ≠ 不存在：Count 二次确认（CR-01）
		var n int64
		if err := r.db.WithContext(ctx).Model(&po.RolePO{}).Where("id = ?", id).Count(&n).Error; err != nil {
			return err
		}
		if n == 0 {
			return errcode.ErrNotFound
		}
	}
	return nil
}

// Delete 物理删除（硬删除语义，P4）；由 service 层 db.Transaction 三连删调用
// （user_role → role_menu → role，任一失败整体回滚）。恒带 Where 防 Type 白名单外调用。
func (r *RoleRepo) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&po.RolePO{}).Error
}

// List 角色分页条件查询：roleName LIKE（**先 strings.TrimSpace**——源
// RoleRepositoryImpl.selectPage 特例）+ **ORDER BY id ASC**（非其他域的
// create_time DESC，源特例 RoleRepositoryImpl.java:28-34；id 递增本身即稳定序）。
// 条件放进闭包 q() 保证 Count 与 Find 一致防 total 漂移（P15）。
func (r *RoleRepo) List(ctx context.Context, page, size int, roleName string) ([]po.RolePO, int64, error) {
	trimmed := strings.TrimSpace(roleName)
	q := func() *gorm.DB {
		qry := r.db.WithContext(ctx).Model(&po.RolePO{})
		if trimmed != "" {
			qry = qry.Where("role_name LIKE ?", "%"+trimmed+"%")
		}
		return qry
	}
	var total int64
	if err := q().Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * size
	if offset < 0 {
		offset = 0
	}
	var roles []po.RolePO
	err := q().Offset(offset).Limit(size).
		Order("id ASC").
		Find(&roles).Error
	if err != nil {
		return nil, 0, err
	}
	return roles, total, nil
}

// FindByID 按主键查询；未命中返回 ErrNotFound（10300）。
func (r *RoleRepo) FindByID(ctx context.Context, id int64) (*po.RolePO, error) {
	var role po.RolePO
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&role).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errcode.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &role, nil
}

// FindByIDs 批量按主键查询（WHERE id IN ?）；供 02-03 用户角色装配等需全量
// 场景使用（源 RoleApplicationService.listByIds 等价）。缺失 id 自然缺席结果集。
func (r *RoleRepo) FindByIDs(ctx context.Context, ids []int64) ([]po.RolePO, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var roles []po.RolePO
	err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&roles).Error
	if err != nil {
		return nil, err
	}
	return roles, nil
}
