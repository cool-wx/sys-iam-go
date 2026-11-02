// sys_api_key 数据访问（登录查询 + CRUD 管理面）。
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

// ApiKeyRepo sys_api_key 数据访问。
type ApiKeyRepo struct {
	db *gorm.DB
}

// NewApiKeyRepo 构造（db 可为 *gorm.DB 或事务 tx）。
func NewApiKeyRepo(db *gorm.DB) *ApiKeyRepo {
	return &ApiKeyRepo{db: db}
}

// FindByKey 登录查询（等价源 ApiKeyRepositoryImpl.getByApiKey）：key 精确匹配 +
// ENABLED + 未过期（expire_time IS NULL 或 > NOW()）；查不到返回 (nil, nil)——
// 无效/停用/过期在 service 层统一译为 ErrApiKeyInvalid（10503）。
// 这是**唯一**登录取 key 路径：停用/过期 key 恒不可登录。
func (r *ApiKeyRepo) FindByKey(ctx context.Context, apiKey string) (*po.ApiKeyPO, error) {
	var k po.ApiKeyPO
	err := r.db.WithContext(ctx).
		Where("api_key = ? AND status = 1 AND (expire_time IS NULL OR expire_time > NOW())", apiKey).
		First(&k).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &k, nil
}

// Create 插入（key/username 冗余/status 默认值由 service 层装配）；审计双时间与
// 四操作人列由 PO BeforeCreate 钩子填充。
func (r *ApiKeyRepo) Create(ctx context.Context, k *po.ApiKeyPO) error {
	return r.db.WithContext(ctx).Create(k).Error
}

// FindByID 按主键查询；未命中返回 ErrNotFound（10300）。
func (r *ApiKeyRepo) FindByID(ctx context.Context, id int64) (*po.ApiKeyPO, error) {
	var k po.ApiKeyPO
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&k).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errcode.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &k, nil
}

// Patch 更新白名单列（存在性判定同 user repo：0 行变更回读 Count 确认，
// 仅当行确实不存在才返回 ErrNotFound）。审计（update_time/update_by 两列）在此
// 显式并入，不接受客户端同名键。service 层仅传 status 单列（源
// editApiKey 单列语义——**任何写路径不得覆写 api_key/username/user_id/expire_time
// 既有值**）。
func (r *ApiKeyRepo) Patch(ctx context.Context, id int64, cols map[string]any) error {
	if cols == nil {
		cols = map[string]any{}
	}
	now := time.Now()
	cols["update_time"] = now
	if identity, ok := auth.FromContext(ctx); ok {
		cols["update_by_id"] = identity.UserID
		cols["update_by_name"] = identity.Name
	}
	res := r.db.WithContext(ctx).Model(&po.ApiKeyPO{}).Where("id = ?", id).Updates(cols)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		// 0 变更 ≠ 不存在：需 Count 确认行是否存在
		var n int64
		if err := r.db.WithContext(ctx).Model(&po.ApiKeyPO{}).Where("id = ?", id).Count(&n).Error; err != nil {
			return err
		}
		if n == 0 {
			return errcode.ErrNotFound
		}
	}
	return nil
}

// List 分页查询（Count + Find 两查询防 total 漂移，P15）；排序 create_time DESC,
// id DESC（稳定 tiebreaker，与 user 域一致）；当前无过滤条件（源 selectPage 无
// 附加条件）。offset<0 钳 0。
func (r *ApiKeyRepo) List(ctx context.Context, page, size int) ([]po.ApiKeyPO, int64, error) {
	q := func() *gorm.DB {
		return r.db.WithContext(ctx).Model(&po.ApiKeyPO{})
	}
	var total int64
	if err := q().Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * size
	if offset < 0 {
		offset = 0
	}
	var keys []po.ApiKeyPO
	err := q().Offset(offset).Limit(size).
		Order("create_time DESC, id DESC").
		Find(&keys).Error
	if err != nil {
		return nil, 0, err
	}
	return keys, total, nil
}

// Delete 物理删除（硬删除语义，P4）；恒带 Where 防 Type 白名单外调用。
func (r *ApiKeyRepo) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&po.ApiKeyPO{}).Error
}
