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
// 保证确定性。绝不在更新路径调用 Save（Pitfall 3）。ctx 含 identity 时写入
// update_by 两列，否则仅刷 update_time。
//
// 存在性判定不以 RowsAffected 为准（CR-01）：go-sql-driver 默认
// clientFoundRows=false，UPDATE 返回的是实际变更行数而非匹配行数；update_time 为
// 秒精度 datetime，同秒内重复提交相同内容（幂等重试/双击保存/PUT {}）时 SET 值
// 全部与现值相等 → 0 行变更但行存在。故 RowsAffected==0 时回读 Count 二次确认，
// 仅当行确实不存在才返回 ErrNotFound。
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
		// 0 变更 ≠ 不存在：需 Count 确认行是否存在（见函数注释 CR-01）
		var n int64
		if err := r.db.WithContext(ctx).Model(&po.UserPO{}).Where("id = ?", id).Count(&n).Error; err != nil {
			return err
		}
		if n == 0 {
			return errcode.ErrNotFound
		}
	}
	return nil
}

// List 分页查询：Count 与记录查询经同一闭包构造（Phase 1 无过滤条件，条件集
// 天然一致，防 total 漂移，P15）；排序 create_time DESC, id DESC——稳定 tiebreaker
// 保证同秒插入时翻页不漂移；排序串为代码常量，无 ORDER 注入面（T-1-10）。
// page/size 已由 paging.Parse 钳位（≥1 且 page ≤ maxPage）；Offset 计算后仍防负
// （int 溢出双保险，WR-03，P8）。
func (r *UserRepo) List(ctx context.Context, page, size int) ([]po.UserPO, int64, error) {
	q := func() *gorm.DB {
		return r.db.WithContext(ctx).Model(&po.UserPO{})
	}
	var total int64
	if err := q().Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * size
	if offset < 0 {
		// int 溢出保护（WR-03，双保险）：GORM clause.Limit 仅在 Offset>0 时写
		// OFFSET 子句，溢出为负会被静默省略 → 静默回退第 1 页数据。钳位后正常
		// 不会触发，防御绕过 paging.Parse 的直接调用。
		offset = 0
	}
	var users []po.UserPO
	err := q().Offset(offset).Limit(size).
		Order("create_time DESC, id DESC").
		Find(&users).Error
	if err != nil {
		return nil, 0, err
	}
	return users, total, nil
}

// FindByUsername 按 username 精确查询（登录六步链第一步）；未命中返回 (nil, nil)——
// "用户不存在"与"密码错误"在 service 层统一译为 ErrBadCredentials（防枚举，T-2-01），
// repo 层不区分这两种语义，故不做 ErrNotFound 翻译。
func (r *UserRepo) FindByUsername(ctx context.Context, username string) (*po.UserPO, error) {
	var u po.UserPO
	err := r.db.WithContext(ctx).Where("username = ?", username).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
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

// Delete 物理删除（硬删除语义，P4）；由 service 层 db.Transaction 调用（先删
// sys_user_role 关联再删用户，源 removeById 顺序）。恒带 Where 防 Type 白名单外调用。
func (r *UserRepo) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&po.UserPO{}).Error
}
