package dto

import (
	"github.com/cool-wx/sys-iam-go/internal/model/po"
	"github.com/cool-wx/sys-iam-go/pkg/ids"
	"github.com/cool-wx/sys-iam-go/pkg/types"
)

// ConfigCreateReq 创建系统配置请求。configKey/configValue 必填由 binding 校验
// 拦截；configType 可选。不得定义任何审计字段键。
type ConfigCreateReq struct {
	ConfigKey   *string `json:"configKey" binding:"required"`
	ConfigValue *string `json:"configValue" binding:"required"`
	ConfigType  *string `json:"configType"`
	Remark      *string `json:"remark"`
}

// ConfigUpdateReq 更新系统配置请求：全指针字段，nil=未提供（不写库），
// 空串=显式清空。白名单收敛于本结构键集。
type ConfigUpdateReq struct {
	ConfigKey   *string `json:"configKey"`
	ConfigValue *string `json:"configValue"`
	ConfigType  *string `json:"configType"`
	Remark      *string `json:"remark"`
}

// ConfigResp 系统配置响应契约。全部键无 omitempty（未赋值字段以 null 键输出）。
type ConfigResp struct {
	ID           ids.ID               `json:"id"`
	ConfigKey    *string              `json:"configKey"`
	ConfigValue  *string              `json:"configValue"`
	ConfigType   *string              `json:"configType"`
	CreateByID   *ids.ID              `json:"createById"`
	CreateByName *string              `json:"createByName"`
	UpdateByID   *ids.ID              `json:"updateById"`
	UpdateByName *string              `json:"updateByName"`
	CreateTime   *types.LocalDateTime `json:"createTime"`
	UpdateTime   *types.LocalDateTime `json:"updateTime"`
	Remark       *string              `json:"remark"`
}

// ToConfigResp PO→DTO 手写转换（int64 ID→契约 ID 类型）。
func ToConfigResp(m *po.SysConfigPO) *ConfigResp {
	r := &ConfigResp{
		ID:           ids.ID(m.ID),
		ConfigKey:    &m.ConfigKey,
		ConfigValue:  &m.ConfigValue,
		ConfigType:   &m.ConfigType,
		CreateByName: m.CreateByName,
		UpdateByName: m.UpdateByName,
		Remark:       m.Remark,
	}
	if m.CreateByID != nil {
		id := ids.ID(*m.CreateByID)
		r.CreateByID = &id
	}
	if m.UpdateByID != nil {
		id := ids.ID(*m.UpdateByID)
		r.UpdateByID = &id
	}
	if m.CreateTime != nil {
		t := types.LocalDateTime(*m.CreateTime)
		r.CreateTime = &t
	}
	if m.UpdateTime != nil {
		t := types.LocalDateTime(*m.UpdateTime)
		r.UpdateTime = &t
	}
	return r
}
