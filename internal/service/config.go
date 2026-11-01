// Package service 系统配置域业务逻辑：CRUD + 按 key 读取（uk_config_key 1062
// 翻译 + key 未命中翻译，CONF-01）。
package service

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/cool-wx/sys-iam-go/internal/model/dto"
	"github.com/cool-wx/sys-iam-go/internal/model/po"
	"github.com/cool-wx/sys-iam-go/internal/repository"
	"github.com/cool-wx/sys-iam-go/pkg/errcode"
	"github.com/cool-wx/sys-iam-go/pkg/paging"
)

// ConfigService 系统配置域服务。
type ConfigService struct {
	repo *repository.SysConfigRepo
}

// NewConfigService 构造。
func NewConfigService(repo *repository.SysConfigRepo) *ConfigService {
	return &ConfigService{repo: repo}
}

// Create 创建配置：uk_config_key 冲突（TranslateError 开启后 MySQL 1062 →
// gorm.ErrDuplicatedKey，P4）译为 400/10400「配置键已存在」（CONF-01，Phase 2
// 惯例代码实证）。审计字段由 PO 钩子服务端填充（ENG-02）。
func (s *ConfigService) Create(ctx context.Context, req *dto.ConfigCreateReq) (*dto.ConfigResp, error) {
	m := &po.SysConfigPO{
		ConfigKey:   *req.ConfigKey,
		ConfigValue: *req.ConfigValue,
		Remark:      req.Remark,
	}
	if req.ConfigType != nil {
		m.ConfigType = *req.ConfigType
	}
	if err := s.repo.Create(ctx, m); err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return nil, errcode.New(errcode.CodeInvalidParam, "配置键已存在")
		}
		return nil, err
	}
	return dto.ToConfigResp(m), nil
}

// Update 更新配置（部分更新，ENG-05）：白名单 config_key/config_value/
// config_type/remark（nil=不写库；空串=显式清空照写）。落库后回读返回最新契约。
func (s *ConfigService) Update(ctx context.Context, id int64, req *dto.ConfigUpdateReq) (*dto.ConfigResp, error) {
	cols := map[string]any{}
	if req.ConfigKey != nil {
		cols["config_key"] = *req.ConfigKey
	}
	if req.ConfigValue != nil {
		cols["config_value"] = *req.ConfigValue
	}
	if req.ConfigType != nil {
		cols["config_type"] = *req.ConfigType
	}
	if req.Remark != nil {
		cols["remark"] = *req.Remark
	}
	if err := s.repo.Patch(ctx, id, cols); err != nil {
		if errors.Is(err, errcode.ErrNotFound) {
			return nil, errcode.New(errcode.CodeNotFound, fmt.Sprintf("配置不存在: %d", id))
		}
		return nil, err
	}
	return s.Get(ctx, id)
}

// Get 配置详情：未命中译为 404/10300「配置不存在: {id}」。
func (s *ConfigService) Get(ctx context.Context, id int64) (*dto.ConfigResp, error) {
	m, err := s.repo.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, errcode.ErrNotFound) {
			return nil, errcode.New(errcode.CodeNotFound, fmt.Sprintf("配置不存在: %d", id))
		}
		return nil, err
	}
	return dto.ToConfigResp(m), nil
}

// GetByKey 按 config_key 读取配置：未命中译为 404/10300「配置不存在: {key}」
// （源 getByKey 语义，消息按域惯例命名）。
func (s *ConfigService) GetByKey(ctx context.Context, key string) (*po.SysConfigPO, error) {
	m, err := s.repo.FindByKey(ctx, key)
	if err != nil {
		if errors.Is(err, errcode.ErrNotFound) {
			return nil, errcode.New(errcode.CodeNotFound, fmt.Sprintf("配置不存在: %s", key))
		}
		return nil, err
	}
	return m, nil
}

// List 配置分页列表（无 dataScope——D-03 拍板零应用）。
func (s *ConfigService) List(ctx context.Context, page, size int) (*paging.PageData[dto.ConfigResp], error) {
	items, total, err := s.repo.List(ctx, page, size)
	if err != nil {
		return nil, err
	}
	out := make([]dto.ConfigResp, 0, len(items))
	for i := range items {
		out = append(out, *dto.ToConfigResp(&items[i]))
	}
	return &paging.PageData[dto.ConfigResp]{Items: out, Total: total, Page: page, Size: size}, nil
}

// Delete 删除配置：直接物理删（源 deleteConfig 语义）。
func (s *ConfigService) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}
