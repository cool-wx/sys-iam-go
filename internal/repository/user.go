// Package repository 定义 GORM 数据访问（薄封装；tx 可作 db 注入以复用事务）。
package repository

import (
	"context"

	"gorm.io/gorm"

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

// Patch 显式列更新（编译存根：RED 阶段未实现真实写路径）。
func (r *UserRepo) Patch(ctx context.Context, id int64, cols map[string]any) error {
	_ = ctx
	_ = id
	_, _ = cols["update_time"]
	return nil
}

// FindByID 按主键查询（编译存根：RED 阶段未实现）。
func (r *UserRepo) FindByID(ctx context.Context, id int64) (*po.UserPO, error) {
	_ = ctx
	_ = id
	return nil, errcode.ErrNotFound
}
