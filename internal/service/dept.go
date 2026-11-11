// Package service 部门域业务逻辑（与菜单域同构）：父校验 + parentName 服务端
// 冗余 + 树查询（树构建与菜单域共用 internal/tree 泛型构建器）。
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

// DeptService 部门域服务。pub 用于改名提交成功后发布 DeptNameChanged 事件
// （冗余名称列同步，发布时序契约见 internal/event 包注释）。
type DeptService struct {
	repo *repository.DeptRepo
	pub  event.Publisher
}

// NewDeptService 构造。
func NewDeptService(repo *repository.DeptRepo, pub event.Publisher) *DeptService {
	return &DeptService{repo: repo, pub: pub}
}

// Create 创建部门：DTO→PO（服务端默认值 status=ENABLED——对齐 DDL default）；
// parentId 非 nil 且非 0 时校验父存在并冗余 parentName
// （服务端查库装配）。
func (s *DeptService) Create(ctx context.Context, req *dto.DeptCreateReq) (*dto.DeptResp, error) {
	d := &po.DeptPO{
		DeptName: req.DeptName,
		Sort:     req.Sort,
		Remark:   req.Remark,
	}
	status := int8(1)
	d.Status = &status
	if req.ParentID != nil && *req.ParentID != 0 {
		pid := int64(*req.ParentID)
		parent, err := s.resolveParent(ctx, pid)
		if err != nil {
			return nil, err
		}
		d.ParentID = &pid
		d.ParentName = parent.DeptName // 服务端查库冗余，不信请求体
	}
	if err := s.repo.Create(ctx, d); err != nil {
		return nil, err
	}
	return dto.ToDeptResp(d), nil
}

// Update 更新部门（部分更新）：白名单 deptName/parentId/sort/status/remark；
// parentId 显式传入时联动父校验 + parentName 重冗余（服务端查库装配）。
// 落库后回读返回最新契约；审计由 repo.Patch 显式并入。
//
// 改名同步（**不设**「新名≠旧名」短路——主行已提交后重试同一请求时
// 旧行已是新名，新≠旧守卫恒假、事件永不重发——「重试幂等自愈」被证伪；显式带
// deptName 即发布，幂等 UPDATE 使同名重发退化为 no-op）：请求显式带 deptName 时先
// 回读旧行（同时把"行不存在"提前拦为 404）；Patch 成功
// （SkipDefaultTransaction 下单语句已提交——「提交后发布」由代码顺序天然满足，
// 发布语句绝不放在 Patch 之前或事务闭包内）之后发布 DeptNameChanged（显式带名门
// 保留，"仅改备注"不带 deptName 仍零发布，非无条件发布）；Publish 失败上抛 500
// （客户端重试同一请求必重发事件，幂等 UPDATE 自愈，沿 casbin 双轨纪律）。
func (s *DeptService) Update(ctx context.Context, id int64, req *dto.DeptUpdateReq) (*dto.DeptResp, error) {
	// 发布条件需要旧名：仅当请求显式带部门名时回读旧行；
	// 行不存在提前 404 拦截，Patch 与发布一个都不触发。
	var old *po.DeptPO
	if req.DeptName != nil {
		var err error
		old, err = s.repo.FindByID(ctx, id)
		if err != nil {
			if errors.Is(err, errcode.ErrNotFound) {
				return nil, errcode.New(errcode.CodeNotFound, fmt.Sprintf("部门不存在: %d", id))
			}
			return nil, err
		}
	}
	cols := map[string]any{}
	if req.DeptName != nil {
		cols["dept_name"] = *req.DeptName
	}
	if req.ParentID != nil {
		pid := int64(*req.ParentID)
		if pid == 0 {
			// 显式移至根：parent_id 落 NULL + parent_name 清 NULL——根 = parent_id
			// NULL 成为唯一形态（与 Create 根同形态）。存量
			// parent_id=0 行不迁移：tree.Build/ensureNotDescendant/resolveParent
			// 对 nil/0 双形态兼容，读侧零回归。GORM Updates(map) 显式 nil 写 NULL
			//（与同分支 parent_name 清 NULL 一致，显式置空防旧值残留，同菜单域）。
			cols["parent_id"] = nil
			cols["parent_name"] = nil
		} else {
			// 成环拦截（与菜单域同构）：自引用/挂到自身子孙均拒绝——落库后
			// 环上节点在 tree.Build 中互不归根，整条子树从树查询静默消失
			if pid == id {
				return nil, errcode.New(errcode.CodeInvalidParam, "上级部门不能是自己")
			}
			if err := s.ensureNotDescendant(ctx, pid, id); err != nil {
				return nil, err
			}
			parent, err := s.resolveParent(ctx, pid)
			if err != nil {
				return nil, err
			}
			cols["parent_id"] = pid
			cols["parent_name"] = *parent.DeptName
		}
	}
	if req.Sort != nil {
		cols["sort"] = *req.Sort
	}
	if req.Status != nil {
		cols["status"] = int8(*req.Status)
	}
	if req.Remark != nil {
		cols["remark"] = *req.Remark
	}
	if err := s.repo.Patch(ctx, id, cols); err != nil {
		if errors.Is(err, errcode.ErrNotFound) {
			return nil, errcode.New(errcode.CodeNotFound, fmt.Sprintf("部门不存在: %d", id))
		}
		return nil, err
	}
	// 改名事件发布（Patch 返回 nil 即已提交；显式带名即发布、
	// 不设新≠旧短路——重试自愈契约见上注）：oldName 取回读行值、NULL 兜底 ""
	// （仅载荷信息非 UPDATE 条件）。
	if old != nil && req.DeptName != nil {
		oldName := ""
		if old.DeptName != nil {
			oldName = *old.DeptName
		}
		evt := event.NameChangedEvent{
			Type:    event.DeptNameChanged,
			ID:      id,
			OldName: oldName,
			NewName: *req.DeptName,
		}
		if err := s.pub.Publish(ctx, evt); err != nil {
			return nil, err // → 500（同步 UPDATE 失败可观察，重试必重发自愈）
		}
	}
	return s.Get(ctx, id)
}

// Get 部门详情：未命中译为 404/10300「部门不存在: {id}」（固定文案；
// code/HTTP 契约全库统一）。
func (s *DeptService) Get(ctx context.Context, id int64) (*dto.DeptResp, error) {
	d, err := s.repo.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, errcode.ErrNotFound) {
			return nil, errcode.New(errcode.CodeNotFound, fmt.Sprintf("部门不存在: %d", id))
		}
		return nil, err
	}
	return dto.ToDeptResp(d), nil
}

// List 部门全量列表（无分页——树查询需要完整数据集；ORDER BY sort ASC 由 repo 承担）。
func (s *DeptService) List(ctx context.Context) ([]*dto.DeptResp, error) {
	items, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*dto.DeptResp, 0, len(items))
	for i := range items {
		out = append(out, dto.ToDeptResp(&items[i]))
	}
	return out, nil
}

// Tree 部门树：List 全量 → tree.Build → []DeptTreeResp。
// 孤儿（parentId 为 nil/0/指向不存在节点）均提升为根——树查询语义
// 由 internal/tree 泛型构建器统一定义。
func (s *DeptService) Tree(ctx context.Context) ([]*dto.DeptTreeResp, error) {
	items, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	respItems := make([]dto.DeptTreeResp, 0, len(items))
	for i := range items {
		respItems = append(respItems, *dto.ToDeptTreeResp(&items[i]))
	}
	roots := tree.Build(respItems,
		func(d dto.DeptTreeResp) int64 { return int64(d.ID) },
		func(d dto.DeptTreeResp) *int64 {
			if d.ParentID == nil {
				return nil
			}
			v := int64(*d.ParentID)
			return &v
		},
		func(node *dto.DeptTreeResp, children []dto.DeptTreeResp) {
			node.Children = make([]*dto.DeptTreeResp, 0, len(children))
			for i := range children {
				c := children[i]
				node.Children = append(node.Children, &c)
			}
		},
	)
	out := make([]*dto.DeptTreeResp, 0, len(roots))
	for i := range roots {
		r := roots[i]
		out = append(out, &r)
	}
	return out, nil
}

// Delete 删除部门：直接物理删。**不校验子部门/用户引用**——删除仍成功，
func (s *DeptService) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

// ensureNotDescendant 成环拦截（与菜单域同构）：把 id 挂到 pid 下前，沿
// pid 的父链向上上溯（visited 集防存量环数据死循环 + 深度上限冗余兜底）——若
// 途中遇到 id，说明 pid 位于 id 的子链上，挂接将成环，报 400。父链中断（节点
// 不存在/到根）即无环。
func (s *DeptService) ensureNotDescendant(ctx context.Context, pid, id int64) error {
	visited := make(map[int64]bool)
	const maxDepth = 100
	cur := pid
	for depth := 0; depth < maxDepth && !visited[cur]; depth++ {
		if cur == id {
			return errcode.New(errcode.CodeInvalidParam, fmt.Sprintf("上级部门不能是当前部门的子孙节点: %d", pid))
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

// resolveParent 父校验：父不存在或父 dept_name
// 为 NULL 均报 400「上级部门不存在: {pid}」；parentName 只信服务端查库值。
func (s *DeptService) resolveParent(ctx context.Context, pid int64) (*po.DeptPO, error) {
	parent, err := s.repo.FindByID(ctx, pid)
	if err != nil {
		if errors.Is(err, errcode.ErrNotFound) {
			return nil, errcode.New(errcode.CodeInvalidParam, fmt.Sprintf("上级部门不存在: %d", pid))
		}
		return nil, err
	}
	if parent.DeptName == nil {
		// 父 dept_name 为 NULL 与父不存在同样拒绝
		return nil, errcode.New(errcode.CodeInvalidParam, fmt.Sprintf("上级部门不存在: %d", pid))
	}
	return parent, nil
}
