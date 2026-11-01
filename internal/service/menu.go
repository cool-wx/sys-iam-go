// Package service 菜单域业务逻辑：父校验 + parentName 服务端冗余 + 树查询
// （tree.Build 孤儿提升语义的唯一消费方之一，MENU-01/02）。
package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/cool-wx/sys-iam-go/internal/event"
	"github.com/cool-wx/sys-iam-go/internal/model/dto"
	"github.com/cool-wx/sys-iam-go/internal/model/po"
	"github.com/cool-wx/sys-iam-go/internal/repository"
	"github.com/cool-wx/sys-iam-go/internal/tree"
	"github.com/cool-wx/sys-iam-go/pkg/errcode"
)

// MenuService 菜单域服务。pub 用于改名事件发布（Phase 5 SYNC-01）：本 plan 一次
// 改齐构造签名，MenuNameChanged 发布挂点在 05-02 实装——字段暂存不用。
type MenuService struct {
	repo *repository.MenuRepo
	pub  event.Publisher
}

// NewMenuService 构造。
func NewMenuService(repo *repository.MenuRepo, pub event.Publisher) *MenuService {
	return &MenuService{repo: repo, pub: pub}
}

// Create 创建菜单：DTO→PO（服务端默认值 status=ENABLED、visible=true——对齐 DDL
// default 与源 MP null-skip 语义）；parentId 非 nil 且非 0 时校验父存在并冗余
// parentName（源 addMenu resolveParentName 语义，MENU-01）。审计字段（双时间/
// 四操作人/ID）由 PO 钩子服务端填充，不接受请求体任何同名客户端字段（ENG-02）。
func (s *MenuService) Create(ctx context.Context, req *dto.MenuCreateReq) (*dto.MenuResp, error) {
	m := &po.MenuPO{
		Name:       req.Name,
		Path:       req.Path,
		Component:  req.Component,
		Permission: req.Permission,
		Icon:       req.Icon,
		Sort:       req.Sort,
		Visible:    req.Visible,
		Remark:     req.Remark,
	}
	if req.Visible == nil {
		v := true
		m.Visible = &v
	}
	status := int8(1)
	m.Status = &status
	if req.Type != nil {
		tp := int8(*req.Type)
		m.Type = &tp
	}
	if req.ParentID != nil && *req.ParentID != 0 {
		pid := int64(*req.ParentID)
		parent, err := s.resolveParent(ctx, pid)
		if err != nil {
			return nil, err
		}
		m.ParentID = &pid
		m.ParentName = parent.Name // 服务端查库冗余，不信请求体（T-1-10 同源纪律）
	}
	// parentId nil/0：parent_id/parent_name 保持 NULL（源语义）
	if err := s.repo.Create(ctx, m); err != nil {
		return nil, err
	}
	return dto.ToMenuResp(m), nil
}

// Update 更新菜单（部分更新，ENG-05）：白名单 name/type/path/component/permission/
// icon/sort/visible/status/remark（nil=不写库；空串=显式清空照写）；parentId 显式
// 传入时联动父校验 + parentName 重冗余（源 editMenu 每次编辑执行 resolveParentName
// 语义）；不显式传则不动父列。落库后回读返回最新契约。审计由 repo.Patch 显式并入
// （ENG-02）。
func (s *MenuService) Update(ctx context.Context, id int64, req *dto.MenuUpdateReq) (*dto.MenuResp, error) {
	cols := map[string]any{}
	if req.Name != nil {
		cols["name"] = *req.Name
	}
	if req.Type != nil {
		cols["type"] = int8(*req.Type)
	}
	if req.ParentID != nil {
		pid := int64(*req.ParentID)
		if pid == 0 {
			// 显式移至根：parent_id=0 + parent_name 清 NULL。
			// （源此处因 MP null-skip 会残留旧 parent_name——有意修正为一致态，
			// 与 Create 的 pid=0 → parent_name=NULL 语义对齐）
			cols["parent_id"] = 0
			cols["parent_name"] = nil
		} else {
			// WR-04 成环拦截：自引用/挂到自身子孙均拒绝——落库后环上节点在
			// tree.Build 中互不归根，整条子树从树查询静默消失（持久脏数据）
			if pid == id {
				return nil, errcode.New(errcode.CodeInvalidParam, "父菜单不能是自己")
			}
			if err := s.ensureNotDescendant(ctx, pid, id); err != nil {
				return nil, err
			}
			parent, err := s.resolveParent(ctx, pid)
			if err != nil {
				return nil, err
			}
			cols["parent_id"] = pid
			cols["parent_name"] = *parent.Name
		}
	}
	if req.Path != nil {
		cols["path"] = *req.Path
	}
	if req.Component != nil {
		cols["component"] = *req.Component
	}
	if req.Permission != nil {
		cols["permission"] = *req.Permission
	}
	if req.Icon != nil {
		cols["icon"] = *req.Icon
	}
	if req.Sort != nil {
		cols["sort"] = *req.Sort
	}
	if req.Visible != nil {
		cols["visible"] = *req.Visible
	}
	if req.Status != nil {
		cols["status"] = int8(*req.Status)
	}
	if req.Remark != nil {
		cols["remark"] = *req.Remark
	}
	if err := s.repo.Patch(ctx, id, cols); err != nil {
		if errors.Is(err, errcode.ErrNotFound) {
			return nil, errcode.New(errcode.CodeNotFound, fmt.Sprintf("菜单不存在: %d", id))
		}
		return nil, err
	}
	return s.Get(ctx, id)
}

// Get 菜单详情：未命中译为 404/10300「菜单不存在: {id}」（源 editMenu 消息原文；
// code/HTTP 契约与 Phase 1 一致）。
func (s *MenuService) Get(ctx context.Context, id int64) (*dto.MenuResp, error) {
	m, err := s.repo.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, errcode.ErrNotFound) {
			return nil, errcode.New(errcode.CodeNotFound, fmt.Sprintf("菜单不存在: %d", id))
		}
		return nil, err
	}
	return dto.ToMenuResp(m), nil
}

// List 菜单全量列表（无分页——树与源 list 语义；ORDER BY sort ASC 由 repo 承担）。
func (s *MenuService) List(ctx context.Context) ([]*dto.MenuResp, error) {
	items, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*dto.MenuResp, 0, len(items))
	for i := range items {
		out = append(out, dto.ToMenuResp(&items[i]))
	}
	return out, nil
}

// Tree 菜单树：List 全量 → tree.Build → []MenuTreeResp。
// 孤儿（parentId 为 nil/0/指向不存在节点）均提升为根——MENU-02 验收语义，
// 与部门树共用 internal/tree 泛型构建器（单点实现，D-13）。
func (s *MenuService) Tree(ctx context.Context) ([]*dto.MenuTreeResp, error) {
	items, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	respItems := make([]dto.MenuTreeResp, 0, len(items))
	for i := range items {
		respItems = append(respItems, *dto.ToMenuTreeResp(&items[i]))
	}
	roots := tree.Build(respItems,
		func(m dto.MenuTreeResp) int64 { return int64(m.ID) },
		func(m dto.MenuTreeResp) *int64 {
			if m.ParentID == nil {
				return nil
			}
			v := int64(*m.ParentID)
			return &v
		},
		func(node *dto.MenuTreeResp, children []dto.MenuTreeResp) {
			node.Children = make([]*dto.MenuTreeResp, 0, len(children))
			for i := range children {
				c := children[i]
				node.Children = append(node.Children, &c)
			}
		},
	)
	out := make([]*dto.MenuTreeResp, 0, len(roots))
	for i := range roots {
		r := roots[i]
		out = append(out, &r)
	}
	return out, nil
}

// Delete 删除菜单：直接物理删。与源一致**不加子菜单/角色引用检查**（源缺口按源
// 保留：MenuApplicationService.removeById 直接删——删除带子节点的菜单仍成功，
// 孤儿子节点经树查询孤儿提升语义仍可见）。
func (s *MenuService) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

// ensureNotDescendant 成环拦截（WR-04）：把 id 挂到 pid 下前，沿 pid 的父链向上
// 上溯（visited 集防存量环数据死循环 + 深度上限冗余兜底）——若途中遇到 id，说明
// pid 位于 id 的子链上，挂接将成环，报 400。父链中断（节点不存在/到根）即无环。
func (s *MenuService) ensureNotDescendant(ctx context.Context, pid, id int64) error {
	visited := make(map[int64]bool)
	const maxDepth = 100
	cur := pid
	for depth := 0; depth < maxDepth && !visited[cur]; depth++ {
		if cur == id {
			return errcode.New(errcode.CodeInvalidParam, fmt.Sprintf("父菜单不能是当前菜单的子孙节点: %d", pid))
		}
		visited[cur] = true
		p, err := s.repo.FindByID(ctx, cur)
		if err != nil || p.ParentID == nil || *p.ParentID == 0 {
			break // 父链中断（不存在/根）→ 无环
		}
		cur = *p.ParentID
	}
	return nil
}

// resolveParent 父校验（源 resolveParentName 逐字语义）：父不存在或父 name 为
// NULL 均报 400「父菜单不存在: {pid}」；parentName 只信服务端查库值。
func (s *MenuService) resolveParent(ctx context.Context, pid int64) (*po.MenuPO, error) {
	parent, err := s.repo.FindByID(ctx, pid)
	if err != nil {
		if errors.Is(err, errcode.ErrNotFound) {
			return nil, errcode.New(errcode.CodeInvalidParam, fmt.Sprintf("父菜单不存在: %d", pid))
		}
		return nil, err
	}
	if parent.Name == nil {
		// 源：parent == null || parent.getName() == null 同样拒绝
		return nil, errcode.New(errcode.CodeInvalidParam, fmt.Sprintf("父菜单不存在: %d", pid))
	}
	return parent, nil
}
