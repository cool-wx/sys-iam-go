// sys_api_key 数据访问（登录查询最小面；写路径随 Wave 2/3 ApiKey 域需求扩展）。
package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/cool-wx/sys-iam-go/internal/model/po"
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
// 无效/停用/过期在 service 层统一译为 ErrApiKeyInvalid（10503，AKEY-02 依赖）。
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
