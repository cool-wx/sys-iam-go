package dto

import (
	"github.com/cool-wx/sys-iam-go/internal/model/po"
	"github.com/cool-wx/sys-iam-go/pkg/ids"
	"github.com/cool-wx/sys-iam-go/pkg/types"
)

// RoleCreateReq 创建角色请求。roleName/roleCode 必填由 binding 校验拦截；
// dataScope/builtin/remark 可选（nil → 服务端默认 dataScope=1/builtin=false，
// 源 Role.create 语义；status 恒默认 ENABLED，创建请求不含 status 键）。
// 不得定义任何审计字段键（ENG-02）。
type RoleCreateReq struct {
	RoleName  *string `json:"roleName" binding:"required"`
	RoleCode  *string `json:"roleCode" binding:"required"`
	DataScope *int8   `json:"dataScope"`
	Builtin   *bool   `json:"builtin"`
	Remark    *string `json:"remark"`
}

// RoleUpdateReq 更新角色请求：全指针字段，nil=未提供（不写库）——源 withEdit
// 逐字段 null-skip 语义；builtin 仅显式传入才覆写。role_code 不在本结构键集
// （角色编码为引用锚点，Go API 契约不开放修改——与 plan DTO 定义一致）。
// 白名单收敛于本结构键集（T-1-10）。
type RoleUpdateReq struct {
	RoleName  *string            `json:"roleName"`
	DataScope *int8              `json:"dataScope"`
	Status    *types.UsageStatus `json:"status"`
	Builtin   *bool              `json:"builtin"`
	Remark    *string            `json:"remark"`
}

// RoleResp 角色响应契约（13 列照 UserResp 形态；dataScope int8 原样——存量
// super_admin 行 data_scope=NULL 以 null 输出）。全部键无 omitempty
// （源 JacksonConfig 契约）；无凭据类字段（02-01 T-2-05 纪律）。
type RoleResp struct {
	ID           ids.ID               `json:"id"`
	RoleName     *string              `json:"roleName"`
	RoleCode     *string              `json:"roleCode"`
	DataScope    *int8                `json:"dataScope"`
	Status       *types.UsageStatus   `json:"status"`
	Builtin      *bool                `json:"builtin"`
	CreateByID   *ids.ID              `json:"createById"`
	CreateByName *string              `json:"createByName"`
	UpdateByID   *ids.ID              `json:"updateById"`
	UpdateByName *string              `json:"updateByName"`
	CreateTime   *types.LocalDateTime `json:"createTime"`
	UpdateTime   *types.LocalDateTime `json:"updateTime"`
	Remark       *string              `json:"remark"`
}

// AssignMenusReq 分配角色菜单请求：menuIds 全量替换语义（空数组=全回收，合法）。
type AssignMenusReq struct {
	MenuIDs []ids.ID `json:"menuIds"`
}

// ToRoleResp PO→DTO 手写转换（枚举经 FromInt 兜底；int64 ID→契约 ID 类型）。
func ToRoleResp(r *po.RolePO) *RoleResp {
	resp := &RoleResp{
		ID:           ids.ID(r.ID),
		RoleName:     &r.RoleName,
		RoleCode:     &r.RoleCode,
		DataScope:    r.DataScope,
		Builtin:      r.Builtin,
		CreateByName: r.CreateByName,
		UpdateByName: r.UpdateByName,
		Remark:       r.Remark,
	}
	if r.Status != nil {
		s := types.UsageStatusFromInt(*r.Status)
		resp.Status = &s
	}
	if r.CreateByID != nil {
		id := ids.ID(*r.CreateByID)
		resp.CreateByID = &id
	}
	if r.UpdateByID != nil {
		id := ids.ID(*r.UpdateByID)
		resp.UpdateByID = &id
	}
	if r.CreateTime != nil {
		t := types.LocalDateTime(*r.CreateTime)
		resp.CreateTime = &t
	}
	if r.UpdateTime != nil {
		t := types.LocalDateTime(*r.UpdateTime)
		resp.UpdateTime = &t
	}
	return resp
}
