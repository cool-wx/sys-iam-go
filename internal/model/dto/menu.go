package dto

import (
	"github.com/cool-wx/sys-iam-go/internal/model/po"
	"github.com/cool-wx/sys-iam-go/pkg/ids"
	"github.com/cool-wx/sys-iam-go/pkg/types"
)

// MenuCreateReq 创建菜单请求。指针字段 nil=未提供；name/type 必填由 binding 校验
// 拦截。不得定义任何审计字段键；parentId 传 0 与不传同义（根）。
type MenuCreateReq struct {
	Name       *string         `json:"name" binding:"required"`
	Type       *types.MenuType `json:"type" binding:"required"`
	ParentID   *ids.ID         `json:"parentId"`
	Path       *string         `json:"path"`
	Component  *string         `json:"component"`
	Permission *string         `json:"permission"`
	Icon       *string         `json:"icon"`
	Sort       *int            `json:"sort"`
	Visible    *bool           `json:"visible"`
	Remark     *string         `json:"remark"`
}

// MenuUpdateReq 更新菜单请求：全指针字段，nil=未提供（不写库），空串=显式清空。
// parentId 显式传入时联动父校验 + parentName 重冗余（服务端查库装配）；
// 不显式传则不动父。白名单收敛于本结构键集；status 经 UsageStatus
// 严格解析——非空未知值 400/10401（收紧：此前的「未知兜底 ENABLED 落库」会把
// 拼错的停用值静默反转回启用），null/缺失落 nil 指针由更新分支承接（不写库）。
type MenuUpdateReq struct {
	Name       *string            `json:"name"`
	Type       *types.MenuType    `json:"type"`
	ParentID   *ids.ID            `json:"parentId"`
	Path       *string            `json:"path"`
	Component  *string            `json:"component"`
	Permission *string            `json:"permission"`
	Icon       *string            `json:"icon"`
	Sort       *int               `json:"sort"`
	Visible    *bool              `json:"visible"`
	Status     *types.UsageStatus `json:"status"`
	Remark     *string            `json:"remark"`
}

// MenuResp 菜单响应契约。
// 全部键无 omitempty——未赋值字段以 null 键输出而非键缺失；
// type 经 types.MenuType name|int 双形态序列化；无凭据类字段。
type MenuResp struct {
	ID           ids.ID               `json:"id"`
	ParentID     *ids.ID              `json:"parentId"`
	ParentName   *string              `json:"parentName"`
	Name         *string              `json:"name"`
	Type         *types.MenuType      `json:"type"`
	Path         *string              `json:"path"`
	Component    *string              `json:"component"`
	Permission   *string              `json:"permission"`
	Icon         *string              `json:"icon"`
	Sort         *int                 `json:"sort"`
	Visible      *bool                `json:"visible"`
	Status       *types.UsageStatus   `json:"status"`
	CreateByID   *ids.ID              `json:"createById"`
	CreateByName *string              `json:"createByName"`
	UpdateByID   *ids.ID              `json:"updateById"`
	UpdateByName *string              `json:"updateByName"`
	CreateTime   *types.LocalDateTime `json:"createTime"`
	UpdateTime   *types.LocalDateTime `json:"updateTime"`
	Remark       *string              `json:"remark"`
}

// MenuTreeResp 树节点：MenuResp 内嵌 + children 递归嵌套。
// Children 恒非 nil（叶子经 tree.Build attach 空切片归一化）——JSON 出口 [] 非 null。
type MenuTreeResp struct {
	MenuResp
	Children []*MenuTreeResp `json:"children"`
}

// ToMenuResp PO→DTO 手写转换（枚举经 FromInt 兜底；int64 ID→契约 ID 类型）。
func ToMenuResp(m *po.MenuPO) *MenuResp {
	r := &MenuResp{
		ID:           ids.ID(m.ID),
		ParentName:   m.ParentName,
		Name:         m.Name,
		Path:         m.Path,
		Component:    m.Component,
		Permission:   m.Permission,
		Icon:         m.Icon,
		Sort:         m.Sort,
		Visible:      m.Visible,
		CreateByName: m.CreateByName,
		UpdateByName: m.UpdateByName,
		Remark:       m.Remark,
	}
	if m.ParentID != nil {
		id := ids.ID(*m.ParentID)
		r.ParentID = &id
	}
	if m.Type != nil {
		mt := types.MenuTypeFromInt(*m.Type)
		r.Type = &mt
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

// ToMenuTreeResp PO→树节点转换：Children 预置空切片（叶子出口 [] 非 null；
// 非叶子由 tree.Build 的 attach 回调替换为真实子树）。
func ToMenuTreeResp(m *po.MenuPO) *MenuTreeResp {
	return &MenuTreeResp{
		MenuResp: *ToMenuResp(m),
		Children: []*MenuTreeResp{},
	}
}
