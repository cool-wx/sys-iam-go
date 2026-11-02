package dto

import (
	"github.com/cool-wx/sys-iam-go/internal/model/po"
	"github.com/cool-wx/sys-iam-go/pkg/ids"
	"github.com/cool-wx/sys-iam-go/pkg/types"
)

// ApiKeyCreateReq 创建 API Key 请求（源 addApiKey 语义）：
// userId 可选（nil=不关联用户，username 冗余为空）；expireTime 可选（nil=永不过期）。
// 不含 apiKey/status 键——key 由服务端生成（32 hex，一次性随响应返回）、status
// 恒默认 ENABLED（源 ApiKey.create 语义）。不得定义任何审计字段键。
type ApiKeyCreateReq struct {
	UserID     *ids.ID              `json:"userId"`
	ExpireTime *types.LocalDateTime `json:"expireTime"`
	Remark     *string              `json:"remark"`
}

// ApiKeyUpdateStatusReq 更新 API Key 状态请求：仅 status 单列
// 语义（源 editApiKey 仅更新 status——停用立即生效：FindByKey 登录过滤恒带
// status=ENABLED）。required 拦截缺失键。
type ApiKeyUpdateStatusReq struct {
	Status *types.UsageStatus `json:"status" binding:"required"`
}

// ApiKeyResp API Key 响应契约（13 列照 UserResp/RoleResp 形态，全部键无 omitempty
// ——源 JacksonConfig 契约）。**ApiKey 明文出网**：仅 create 响应一次性返回与
// 管理面 list/get 可见（源语义；明文存储本身为源 schema 语义，威胁评审接受）。
// 任何写路径不得重新生成或覆写既有 key（本 DTO 无写入路径，key 仅在
// service Create 生成）。
type ApiKeyResp struct {
	ID           ids.ID               `json:"id"`
	ApiKey       string               `json:"apiKey"`
	UserID       *ids.ID              `json:"userId"`
	Username     *string              `json:"username"`
	ExpireTime   *types.LocalDateTime `json:"expireTime"`
	Status       *types.UsageStatus   `json:"status"`
	CreateByID   *ids.ID              `json:"createById"`
	CreateByName *string              `json:"createByName"`
	UpdateByID   *ids.ID              `json:"updateById"`
	UpdateByName *string              `json:"updateByName"`
	CreateTime   *types.LocalDateTime `json:"createTime"`
	UpdateTime   *types.LocalDateTime `json:"updateTime"`
	Remark       *string              `json:"remark"`
}

// ToApiKeyResp PO→DTO 手写转换（枚举经 FromInt 兜底；int64 ID→契约 ID 类型）。
func ToApiKeyResp(k *po.ApiKeyPO) *ApiKeyResp {
	r := &ApiKeyResp{
		ID:           ids.ID(k.ID),
		ApiKey:       k.ApiKey,
		Username:     &k.Username,
		ExpireTime:   nil,
		CreateByName: k.CreateByName,
		UpdateByName: k.UpdateByName,
		Remark:       k.Remark,
	}
	if k.UserID != nil {
		id := ids.ID(*k.UserID)
		r.UserID = &id
	}
	if k.Status != nil {
		s := types.UsageStatusFromInt(*k.Status)
		r.Status = &s
	}
	if k.ExpireTime != nil {
		t := types.LocalDateTime(*k.ExpireTime)
		r.ExpireTime = &t
	}
	if k.CreateByID != nil {
		id := ids.ID(*k.CreateByID)
		r.CreateByID = &id
	}
	if k.UpdateByID != nil {
		id := ids.ID(*k.UpdateByID)
		r.UpdateByID = &id
	}
	if k.CreateTime != nil {
		t := types.LocalDateTime(*k.CreateTime)
		r.CreateTime = &t
	}
	if k.UpdateTime != nil {
		t := types.LocalDateTime(*k.UpdateTime)
		r.UpdateTime = &t
	}
	return r
}
