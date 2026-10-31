// Package repository 定义 GORM 数据访问（薄封装；tx 可作 db 注入以复用事务）。
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

// UserRepo sys_user 数据访问。
type UserRepo struct {
	db *gorm.DB
}

// NewUserRepo 构造（db 可为 *gorm.DB 或事务 tx）。
func NewUserRepo(db *gorm.DB) *UserRepo {
	return &UserRepo{db: db}
}

// Create 插入一行；ctx 经 WithContext 传递，供审计钩子读取 identity。
// 参数化由 GORM 占位符保证，不拼 SQL 字符串（T-1-03）。
func (r *UserRepo) Create(ctx context.Context, u *po.UserPO) error {
	return r.db.WithContext(ctx).Create(u).Error
}

// Patch 显式列更新（零值保护核心，ENG-05）：只写 cols 中出现的列（白名单收敛于
// 调用方构造的 map 键，请求体额外键不可能进入，T-1-10）；恒带 Where("id = ?") 防
// 全表更新（T-1-08）。update_time 与审计操作人列在此显式并入 map——GORM 对
// Updates(map) 不会把钩子改动的结构体字段写进 SET 子句（已知语义坑），显式并入
// 保证确定性。绝不在更新路径调用 Save（Pitfall 3）。RowsAffected==0 → ErrNotFound
// （防"静默无效果"）。ctx 含 identity 时写入 update_by 两列，否则仅刷 update_time。
func (r *UserRepo) Patch(ctx context.Context, id int64, cols map[string]any) error {
	if cols == nil {
		cols = map[string]any{}
	}
	now := time.Now()
	cols["update_time"] = now
	if identity, ok := auth.FromContext(ctx); ok {
		cols["update_by_id"] = identity.UserID
		cols["update_by_name"] = identity.Name
	}
	res := r.db.WithContext(ctx).Model(&po.UserPO{}).Where("id = ?", id).Updates(cols)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errcode.ErrNotFound
	}
	return nil
}

// FindByID 按主键查询；未命中返回 ErrNotFound（10300）。
func (r *UserRepo) FindByID(ctx context.Context, id int64) (*po.UserPO, error) {
	var u po.UserPO
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errcode.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}
