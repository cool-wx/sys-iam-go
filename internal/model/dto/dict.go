package dto

import (
	"github.com/cool-wx/sys-iam-go/internal/model/po"
	"github.com/cool-wx/sys-iam-go/pkg/ids"
	"github.com/cool-wx/sys-iam-go/pkg/types"
)

// DictTypeCreateReq 创建字典类型请求。指针字段 nil=未提供；dictName/dictType 必填
// 由 binding 校验拦截；status 不接受客户端传入（服务端默认 1 正常，同源纪律）。
// 不得定义任何审计字段键。
type DictTypeCreateReq struct {
	DictName *string `json:"dictName" binding:"required"`
	DictType *string `json:"dictType" binding:"required"`
	Remark   *string `json:"remark"`
}

// DictTypeUpdateReq 更新字典类型请求：全指针字段，nil=未提供（不写库），
// 空串=显式清空。白名单收敛于本结构键集；status 经 UsageStatus 严格解析
// （先例：未知值 400/10401）。
type DictTypeUpdateReq struct {
	DictName *string            `json:"dictName"`
	DictType *string            `json:"dictType"`
	Status   *types.UsageStatus `json:"status"`
	Remark   *string            `json:"remark"`
}

// DictTypeResp 字典类型响应契约。全部键无 omitempty——未赋值字段以 null 键输出
// 而非键缺失（源 JacksonConfig 契约）；无凭据类字段。
type DictTypeResp struct {
	ID           ids.ID               `json:"id"`
	DictName     *string              `json:"dictName"`
	DictType     *string              `json:"dictType"`
	Status       *types.UsageStatus   `json:"status"`
	CreateByID   *ids.ID              `json:"createById"`
	CreateByName *string              `json:"createByName"`
	UpdateByID   *ids.ID              `json:"updateById"`
	UpdateByName *string              `json:"updateByName"`
	CreateTime   *types.LocalDateTime `json:"createTime"`
	UpdateTime   *types.LocalDateTime `json:"updateTime"`
	Remark       *string              `json:"remark"`
}

// ToDictTypeResp PO→DTO 手写转换（枚举经 FromInt 兜底；int64 ID→契约 ID 类型）。
func ToDictTypeResp(m *po.DictTypePO) *DictTypeResp {
	r := &DictTypeResp{
		ID:           ids.ID(m.ID),
		DictName:     &m.DictName,
		DictType:     &m.DictType,
		CreateByName: m.CreateByName,
		UpdateByName: m.UpdateByName,
		Remark:       m.Remark,
	}
	if m.Status != nil {
		s := types.UsageStatusFromInt(*m.Status)
		r.Status = &s
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

// DictDataCreateReq 创建字典数据请求。dictTypeId 必填（service 前置类型存在性校验，
// 缺失返回 404「字典类型不存在: {id}」源消息原文）；label/value/sort 可选；
// dictTypeName 不接受客户端传入——服务端查库冗余写入。
type DictDataCreateReq struct {
	DictTypeID *ids.ID `json:"dictTypeId" binding:"required"`
	Label      *string `json:"label"`
	Value      *string `json:"value"`
	Sort       *int    `json:"sort"`
	Remark     *string `json:"remark"`
}

// DictDataUpdateReq 更新字典数据请求：全指针字段，nil=未提供（不写库）。
// dictTypeId 显式传入时联动类型校验 + dictTypeName 重冗余（源 editDictData 语义）；
// 不显式传则不动类型列。白名单收敛于本结构键集。
type DictDataUpdateReq struct {
	DictTypeID *ids.ID `json:"dictTypeId"`
	Label      *string `json:"label"`
	Value      *string `json:"value"`
	Sort       *int    `json:"sort"`
	Remark     *string `json:"remark"`
}

// DictDataResp 字典数据响应契约。全部键无 omitempty（源 JacksonConfig 契约）。
type DictDataResp struct {
	ID           ids.ID               `json:"id"`
	DictTypeID   *ids.ID              `json:"dictTypeId"`
	DictTypeName *string              `json:"dictTypeName"`
	Label        *string              `json:"label"`
	Value        *string              `json:"value"`
	Sort         *int                 `json:"sort"`
	CreateByID   *ids.ID              `json:"createById"`
	CreateByName *string              `json:"createByName"`
	UpdateByID   *ids.ID              `json:"updateById"`
	UpdateByName *string              `json:"updateByName"`
	CreateTime   *types.LocalDateTime `json:"createTime"`
	UpdateTime   *types.LocalDateTime `json:"updateTime"`
	Remark       *string              `json:"remark"`
}

// ToDictDataResp PO→DTO 手写转换（int64 ID→契约 ID 类型）。
func ToDictDataResp(m *po.DictDataPO) *DictDataResp {
	r := &DictDataResp{
		ID:           ids.ID(m.ID),
		DictTypeName: m.DictTypeName,
		Label:        m.Label,
		Value:        m.Value,
		Sort:         m.Sort,
		CreateByName: m.CreateByName,
		UpdateByName: m.UpdateByName,
		Remark:       m.Remark,
	}
	tid := ids.ID(m.DictTypeID)
	r.DictTypeID = &tid
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
