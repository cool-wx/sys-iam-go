package dto

import (
	"github.com/cool-wx/sys-iam-go/internal/model/po"
	"github.com/cool-wx/sys-iam-go/pkg/ids"
	"github.com/cool-wx/sys-iam-go/pkg/types"
)

// FaultCreateReq 创建故障信息请求。code/name 必填由 binding 校验拦截；bitIndex/
// type 可选（服务端默认 -1/3——GORM nil 指针写 NULL 会绕过 DB 列默认，语义由
// service 层承担）。
type FaultCreateReq struct {
	Code       *string          `json:"code" binding:"required"`
	Name       *string          `json:"name" binding:"required"`
	BitIndex   *int             `json:"bitIndex"`
	Type       *types.FaultType `json:"type"`
	DictDataID *ids.ID          `json:"dictDataId"`
	Detail     *string          `json:"detail"`
	Reason     *string          `json:"reason"`
	Location   *string          `json:"location"`
	Fix        *string          `json:"fix"`
	Remark     *string          `json:"remark"`
}

// FaultUpdateReq 更新故障信息请求：全指针字段，nil=未提供（不写库），
// 空串=显式清空。白名单收敛于本结构键集。
type FaultUpdateReq struct {
	Code       *string          `json:"code"`
	Name       *string          `json:"name"`
	BitIndex   *int             `json:"bitIndex"`
	Type       *types.FaultType `json:"type"`
	DictDataID *ids.ID          `json:"dictDataId"`
	Detail     *string          `json:"detail"`
	Reason     *string          `json:"reason"`
	Location   *string          `json:"location"`
	Fix        *string          `json:"fix"`
	Remark     *string          `json:"remark"`
}

// FaultResp 故障信息响应契约。type 经 types.FaultType name|int 双形态序列化
// （照 menu_type Resp 惯例）；全部键无 omitempty（源 JacksonConfig 契约）。
type FaultResp struct {
	ID           ids.ID               `json:"id"`
	Code         *string              `json:"code"`
	Name         *string              `json:"name"`
	BitIndex     *int                 `json:"bitIndex"`
	Type         *types.FaultType     `json:"type"`
	DictDataID   *ids.ID              `json:"dictDataId"`
	Detail       *string              `json:"detail"`
	Reason       *string              `json:"reason"`
	Location     *string              `json:"location"`
	Fix          *string              `json:"fix"`
	CreateByID   *ids.ID              `json:"createById"`
	CreateByName *string              `json:"createByName"`
	UpdateByID   *ids.ID              `json:"updateById"`
	UpdateByName *string              `json:"updateByName"`
	CreateTime   *types.LocalDateTime `json:"createTime"`
	UpdateTime   *types.LocalDateTime `json:"updateTime"`
	Remark       *string              `json:"remark"`
}

// ToFaultResp PO→DTO 手写转换（int64 ID→契约 ID 类型；type 经 FaultTypeFromInt
// 兜底 UNKNOWN——源 fromValue 读侧语义）。
func ToFaultResp(m *po.FaultInfoPO) *FaultResp {
	r := &FaultResp{
		ID:           ids.ID(m.ID),
		Code:         m.Code,
		Name:         m.Name,
		BitIndex:     m.BitIndex,
		DictDataID:   (*ids.ID)(m.DictDataID),
		Detail:       m.Detail,
		Reason:       m.Reason,
		Location:     m.Location,
		Fix:          m.Fix,
		CreateByName: m.CreateByName,
		UpdateByName: m.UpdateByName,
		Remark:       m.Remark,
	}
	if m.Type != nil {
		ft := types.FaultTypeFromInt(*m.Type)
		r.Type = &ft
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

// ImportResult Excel 导入结果契约（源 ImportResult DTO，FEATURES.md:205）：
// JSON 键 insertCount/updateCount。
type ImportResult struct {
	InsertCount int `json:"insertCount"`
	UpdateCount int `json:"updateCount"`
}
