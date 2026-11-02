package dto

import (
	"github.com/cool-wx/sys-iam-go/internal/model/po"
	"github.com/cool-wx/sys-iam-go/pkg/ids"
	"github.com/cool-wx/sys-iam-go/pkg/types"
)

// ShiftCreateReq 创建排班请求。shiftCode/shiftName 必填由 binding 校验拦截；
// startTime/endTime 以 HH:MM:SS 字符串直传（不加格式校验不发明格式
// 约束，非法串由 DB 报错走既有翻译路径）；status 不收客户端值，服务端默认
// 1 正常（对齐 dict/fileCategory 域 Create 形态，UsageStatus 枚举纪律）。
type ShiftCreateReq struct {
	ShiftCode *string `json:"shiftCode" binding:"required"`
	ShiftName *string `json:"shiftName" binding:"required"`
	StartTime *string `json:"startTime"`
	EndTime   *string `json:"endTime"`
	Remark    *string `json:"remark"`
}

// ShiftUpdateReq 更新排班请求：全指针字段，nil=未提供（不写库），
// 空串=显式清空。白名单收敛于本结构键集；status 经 UsageStatus 严格
// 解析（未知值 400/10401，不再任意 int8 静默落库）。
type ShiftUpdateReq struct {
	ShiftCode *string            `json:"shiftCode"`
	ShiftName *string            `json:"shiftName"`
	StartTime *string            `json:"startTime"`
	EndTime   *string            `json:"endTime"`
	Status    *types.UsageStatus `json:"status"`
	Remark    *string            `json:"remark"`
}

// ShiftResp 排班响应契约。全部键无 omitempty（源 JacksonConfig 契约）；status
// 为 UsageStatus 枚举形态（出网 "ENABLED"/"DISABLED"，对齐全库纪律）。
type ShiftResp struct {
	ID           ids.ID               `json:"id"`
	ShiftCode    *string              `json:"shiftCode"`
	ShiftName    *string              `json:"shiftName"`
	StartTime    *string              `json:"startTime"`
	EndTime      *string              `json:"endTime"`
	Status       *types.UsageStatus   `json:"status"`
	CreateByID   *ids.ID              `json:"createById"`
	CreateByName *string              `json:"createByName"`
	UpdateByID   *ids.ID              `json:"updateById"`
	UpdateByName *string              `json:"updateByName"`
	CreateTime   *types.LocalDateTime `json:"createTime"`
	UpdateTime   *types.LocalDateTime `json:"updateTime"`
	Remark       *string              `json:"remark"`
}

// ToShiftResp PO→DTO 手写转换（int64 ID→契约 ID 类型；status 经 FromInt 兜底，
// 与 ToDictTypeResp/ToFileCategoryResp 同构）。
func ToShiftResp(m *po.ShiftPO) *ShiftResp {
	r := &ShiftResp{
		ID:           ids.ID(m.ID),
		ShiftCode:    m.ShiftCode,
		ShiftName:    m.ShiftName,
		StartTime:    m.StartTime,
		EndTime:      m.EndTime,
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
