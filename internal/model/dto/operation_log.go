package dto

import (
	"github.com/cool-wx/sys-iam-go/internal/model/po"
	"github.com/cool-wx/sys-iam-go/pkg/ids"
	"github.com/cool-wx/sys-iam-go/pkg/types"
)

// OlogResp 操作日志响应契约（查询域只读）。全部键无 omitempty
// （源 JacksonConfig 契约）；UserID 为 *ids.ID（NULL 语义保留——未登录路由
// 如 login 自身落 NULL）。
type OlogResp struct {
	ID         *ids.ID              `json:"id"`
	UserID     *ids.ID              `json:"userId"`
	RealName   *string              `json:"realName"`
	Module     *string              `json:"module"`
	Operation  *string              `json:"operation"`
	Method     *string              `json:"method"`
	Params     *string              `json:"params"`
	IP         *string              `json:"ip"`
	Duration   *int                 `json:"duration"`
	Status     *int8                `json:"status"`
	ErrorMsg   *string              `json:"errorMsg"`
	CreateTime *types.LocalDateTime `json:"createTime"`
	UpdateTime *types.LocalDateTime `json:"updateTime"`
	Remark     *string              `json:"remark"`
}

// ToOlogResp PO→DTO 手写转换（int64 ID→契约 ID 类型；时间 LocalDateTime）。
func ToOlogResp(m *po.OperationLogPO) *OlogResp {
	id := ids.ID(m.ID)
	r := &OlogResp{
		ID:       &id,
		RealName: m.RealName,
		Module:   m.Module,
		// Operation 路由声明的方法名（声明制，如 "create"/"login"）。
		Operation: m.Operation,
		Method:    m.Method,
		Params:    m.Params,
		IP:        m.IP,
		Duration:  m.Duration,
		Status:    m.Status,
		ErrorMsg:  m.ErrorMsg,
		Remark:    m.Remark,
	}
	if m.UserID != nil {
		uid := ids.ID(*m.UserID)
		r.UserID = &uid
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
