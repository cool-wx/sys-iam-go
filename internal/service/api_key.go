// Package service API Key 域业务逻辑：CRUD + 明文 key 一次性返回（AKEY-01）+
// 停用/过期不可登录（AKEY-02——登录侧由 AuthService.ApikeyLogin + FindByKey
// 恒带过滤保证，本域管理面负责 key 的全生命周期）。
package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/cool-wx/sys-iam-go/internal/model/dto"
	"github.com/cool-wx/sys-iam-go/internal/model/po"
	"github.com/cool-wx/sys-iam-go/internal/repository"
	"github.com/cool-wx/sys-iam-go/pkg/errcode"
	"github.com/cool-wx/sys-iam-go/pkg/paging"
	"github.com/cool-wx/sys-iam-go/pkg/types"
)

// ApiKeyService API Key 域服务。
type ApiKeyService struct {
	userRepo   *repository.UserRepo
	apiKeyRepo *repository.ApiKeyRepo
}

// NewApiKeyService 构造（与 AuthService 复用同一 apiKeyRepo 实例）。
func NewApiKeyService(userRepo *repository.UserRepo, apiKeyRepo *repository.ApiKeyRepo) *ApiKeyService {
	return &ApiKeyService{userRepo: userRepo, apiKeyRepo: apiKeyRepo}
}

// apiKeyNotFound 行不存在译域消息（repo ErrNotFound 哨兵 → 404/10300
// 「API Key 不存在: {id}」——源消息原文）。
func apiKeyNotFound(id int64) *errcode.Error {
	return errcode.New(errcode.CodeNotFound, fmt.Sprintf("API Key 不存在: %d", id))
}

// generateKey 生成 32 位小写 hex 明文 key（crypto/rand 16 字节 hex——等价源
// UUID.randomUUID().replace("-","") 的 128-bit 熵，零依赖）。生成失败收敛为
// 内部错误（不 500 裸抛，T-1-01 纪律）。
func generateKey() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Create 创建 API Key（AKEY-01，源 addApiKey 语义）：userId 非 nil 时校验用户
// 存在（「用户不存在: {id}」）→ username 冗余 **取 user.Username 非 realName**
// （源语义——与 sys_user_role.username 列存 realName 的命名错位相反，勿混）；
// key 服务端生成 32 hex；status 恒默认 ENABLED；明文 key 仅随本响应一次性返回
// （T-2-15：不落任何日志，管理面 list/get 亦可见为源语义）。
func (s *ApiKeyService) Create(ctx context.Context, req *dto.ApiKeyCreateReq) (*dto.ApiKeyResp, error) {
	k := &po.ApiKeyPO{}
	key, err := generateKey()
	if err != nil {
		return nil, errcode.New(errcode.CodeInternal, "密钥生成失败")
	}
	k.ApiKey = key
	if req.UserID != nil {
		u, err := s.userRepo.FindByID(ctx, int64(*req.UserID))
		if err != nil {
			if errors.Is(err, errcode.ErrNotFound) {
				// userId 是请求参数引用的实体（非操作主体）——按域内惯例译
				// 400 CodeInvalidParam（同「部门不存在/角色不存在」链），消息原文保留
				return nil, errcode.New(errcode.CodeInvalidParam,
					fmt.Sprintf("用户不存在: %d", int64(*req.UserID)))
			}
			return nil, err
		}
		k.UserID = &u.ID
		if u.Username != nil {
			k.Username = *u.Username
		}
	}
	if req.ExpireTime != nil {
		t := time.Time(*req.ExpireTime)
		k.ExpireTime = &t
	}
	status := int8(types.UsageEnabled)
	k.Status = &status
	k.Remark = req.Remark
	if err := s.apiKeyRepo.Create(ctx, k); err != nil {
		return nil, err
	}
	return dto.ToApiKeyResp(k), nil
}

// UpdateStatus 更新 API Key 状态（源 editApiKey 仅 status 单列语义）：行不存在
// 译「API Key 不存在: {id}」；停用立即生效（FindByKey 登录过滤恒带
// status=ENABLED——AKEY-02）。api_key/username/user_id/expire_time 既有值不得
// 经本路径或任何写路径覆写（T-2-15）。
func (s *ApiKeyService) UpdateStatus(ctx context.Context, id int64, status types.UsageStatus) error {
	err := s.apiKeyRepo.Patch(ctx, id, map[string]any{"status": int8(status)})
	if err != nil {
		if errors.Is(err, errcode.ErrNotFound) {
			return apiKeyNotFound(id)
		}
		return err
	}
	return nil
}

// List API Key 分页列表：repo 分页查询 → PO→DTO 转换 → paging.PageData envelope
// （排序与钳位由 repo/paging 承担，P15）。管理面可见明文 key（源语义）。
func (s *ApiKeyService) List(ctx context.Context, page, size int) (*paging.PageData[dto.ApiKeyResp], error) {
	keys, total, err := s.apiKeyRepo.List(ctx, page, size)
	if err != nil {
		return nil, err
	}
	items := make([]dto.ApiKeyResp, 0, len(keys))
	for i := range keys {
		items = append(items, *dto.ToApiKeyResp(&keys[i]))
	}
	return &paging.PageData[dto.ApiKeyResp]{Items: items, Total: total, Page: page, Size: size}, nil
}

// Get API Key 详情：按主键查询后 PO→DTO（未命中 404「API Key 不存在: {id}」）。
func (s *ApiKeyService) Get(ctx context.Context, id int64) (*dto.ApiKeyResp, error) {
	k, err := s.apiKeyRepo.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, errcode.ErrNotFound) {
			return nil, apiKeyNotFound(id)
		}
		return nil, err
	}
	return dto.ToApiKeyResp(k), nil
}

// Delete 删除 API Key（物理删，标准三段）：FindByID 校验（行不存在译
// 「API Key 不存在: {id}」）→ 物理删除。
func (s *ApiKeyService) Delete(ctx context.Context, id int64) error {
	if _, err := s.apiKeyRepo.FindByID(ctx, id); err != nil {
		if errors.Is(err, errcode.ErrNotFound) {
			return apiKeyNotFound(id)
		}
		return err
	}
	return s.apiKeyRepo.Delete(ctx, id)
}
