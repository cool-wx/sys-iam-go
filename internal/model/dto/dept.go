package dto

import (
	"github.com/cool-wx/sys-iam-go/internal/model/po"
	"github.com/cool-wx/sys-iam-go/pkg/ids"
	"github.com/cool-wx/sys-iam-go/pkg/types"
)

// DeptCreateReq 创建部门请求（与菜单域同构，dept_name 替 name 族，无 type/path
// 等菜单专属列）。指针字段 nil=未提供；deptName 必填由 binding 校验拦截。
// 不得定义任何审计字段键（ENG-02）；parentId 传 0 与不传同义（根）。
type DeptCreateReq struct {
	DeptName *string `json:"deptName" binding:"required"`
	ParentID *ids.ID `json:"parentId"`
	Sort     *int    `json:"sort"`
	Remark   *string `json:"remark"`
}

// DeptUpdateReq 更新部门请求：全指针字段，nil=未提供（不写库），空串=显式清空。
// parentId 显式传入时联动父校验 + parentName 重冗余（源 editDept resolveParentName
// 同构语义）；白名单收敛于本结构键集（T-1-10）。
type DeptUpdateReq struct {
	DeptName *string            `json:"deptName"`
	ParentID *ids.ID            `json:"parentId"`
	Sort     *int               `json:"sort"`
	Status   *types.UsageStatus `json:"status"`
	Remark   *string            `json:"remark"`
}

// DeptResp 部门响应契约。全部键无 omitempty（源 JacksonConfig 契约）；
// 无凭据类字段（02-01 T-2-05 纪律）。
type DeptResp struct {
	ID           ids.ID               `json:"id"`
	ParentID     *ids.ID              `json:"parentId"`
	ParentName   *string              `json:"parentName"`
	DeptName     *string              `json:"deptName"`
	Sort         *int                 `json:"sort"`
	Status       *types.UsageStatus   `json:"status"`
	CreateByID   *ids.ID              `json:"createById"`
	CreateByName *string              `json:"createByName"`
	UpdateByID   *ids.ID              `json:"updateById"`
	UpdateByName *string              `json:"updateByName"`
	CreateTime   *types.LocalDateTime `json:"createTime"`
	UpdateTime   *types.LocalDateTime `json:"updateTime"`
	Remark       *string              `json:"remark"`
}

// DeptTreeResp 树节点：DeptResp 内嵌 + children 递归嵌套。
// Children 恒非 nil（叶子经 tree.Build attach 空切片归一化）——JSON 出口 [] 非 null。
type DeptTreeResp struct {
	DeptResp
	Children []*DeptTreeResp `json:"children"`
}

// ToDeptResp PO→DTO 手写转换（枚举经 FromInt 兜底；int64 ID→契约 ID 类型）。
func ToDeptResp(d *po.DeptPO) *DeptResp {
	r := &DeptResp{
		ID:           ids.ID(d.ID),
		ParentName:   d.ParentName,
		DeptName:     d.DeptName,
		Sort:         d.Sort,
		CreateByName: d.CreateByName,
		UpdateByName: d.UpdateByName,
		Remark:       d.Remark,
	}
	if d.ParentID != nil {
		id := ids.ID(*d.ParentID)
		r.ParentID = &id
	}
	if d.Status != nil {
		s := types.UsageStatusFromInt(*d.Status)
		r.Status = &s
	}
	if d.CreateByID != nil {
		id := ids.ID(*d.CreateByID)
		r.CreateByID = &id
	}
	if d.UpdateByID != nil {
		id := ids.ID(*d.UpdateByID)
		r.UpdateByID = &id
	}
	if d.CreateTime != nil {
		t := types.LocalDateTime(*d.CreateTime)
		r.CreateTime = &t
	}
	if d.UpdateTime != nil {
		t := types.LocalDateTime(*d.UpdateTime)
		r.UpdateTime = &t
	}
	return r
}

// ToDeptTreeResp PO→树节点转换：Children 预置空切片（叶子出口 [] 非 null；
// 非叶子由 tree.Build 的 attach 回调替换为真实子树）。
func ToDeptTreeResp(d *po.DeptPO) *DeptTreeResp {
	return &DeptTreeResp{
		DeptResp: *ToDeptResp(d),
		Children: []*DeptTreeResp{},
	}
}
