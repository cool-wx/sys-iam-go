package dto

import (
	"github.com/cool-wx/sys-iam-go/internal/model/po"
	"github.com/cool-wx/sys-iam-go/pkg/ids"
	"github.com/cool-wx/sys-iam-go/pkg/types"
)

// RoleCreateReq 创建角色请求。roleName/roleCode 必填由 binding 校验拦截；
// dataScope/remark 可选（nil → 服务端默认 dataScope=1，源 Role.create 语义；
// status 恒默认 ENABLED，创建请求不含 status 键）。不得定义任何审计字段键（ENG-02）。
// builtin 键保留仅用于拒绝检测（WR-01：builtin=true 显式 400——builtin 只能由
// seed/迁移流程设定，防 role:add 制造"永生角色"绕过删除守卫）；服务端恒落 false。
type RoleCreateReq struct {
	RoleName  *string `json:"roleName" binding:"required"`
	RoleCode  *string `json:"roleCode" binding:"required"`
	DataScope *int8   `json:"dataScope"`
	Builtin   *bool   `json:"builtin"`
	Remark    *string `json:"remark"`
}

// RoleUpdateReq 更新角色请求：全指针字段，nil=未提供（不写库）——源 withEdit
// 逐字段 null-skip 语义。builtin 不在本结构键集（WR-01：不可经 edit 翻转——否则
// role:edit 持有者可先翻转 builtin=false 再删除 super_admin，builtin 删除守卫
// 被整体绕行；请求体 builtin 键被忽略，恒不进 UPDATE）。role_code 不在本结构键集
// （角色编码为引用锚点，Go API 契约不开放修改——与 plan DTO 定义一致）。
// 白名单收敛于本结构键集（T-1-10）。
type RoleUpdateReq struct {
	RoleName  *string            `json:"roleName"`
	DataScope *int8              `json:"dataScope"`
	Status    *types.UsageStatus `json:"status"`
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

// AssignScopesReq 分配角色数据权限范围部门请求（D-10 档位 4 存储面）：deptIds
// 全量替换语义（空数组=全回收，合法）——与 AssignMenusReq 同构。
type AssignScopesReq struct {
	DeptIDs []ids.ID `json:"deptIds"`
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
