// Package service 部门域业务逻辑（与菜单域同构）：父校验 + parentName 服务端
// 冗余 + 树查询（DEPT-01/02；树构建与菜单域共用 internal/tree 泛型构建器）。
package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/cool-wx/sys-iam-go/internal/model/dto"
	"github.com/cool-wx/sys-iam-go/internal/model/po"
	"github.com/cool-wx/sys-iam-go/internal/repository"
	"github.com/cool-wx/sys-iam-go/internal/tree"
	"github.com/cool-wx/sys-iam-go/pkg/errcode"
)

// DeptService 部门域服务。
type DeptService struct {
	repo *repository.DeptRepo
}

// NewDeptService 构造。
func NewDeptService(repo *repository.DeptRepo) *DeptService {
	return &DeptService{repo: repo}
}

// Create 创建部门：DTO→PO（服务端默认值 status=ENABLED——对齐 DDL default 与源
// MP null-skip 语义）；parentId 非 nil 且非 0 时校验父存在并冗余 parentName
// （源 addDept resolveParentName 同构语义，DEPT-01）。
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

// Update 更新部门（部分更新，ENG-05）：白名单 deptName/parentId/sort/status/remark；
// parentId 显式传入时联动父校验 + parentName 重冗余（源 editDept 同构语义）。
// 落库后回读返回最新契约；审计由 repo.Patch 显式并入（ENG-02）。
func (s *DeptService) Update(ctx context.Context, id int64, req *dto.DeptUpdateReq) (*dto.DeptResp, error) {
	cols := map[string]any{}
	if req.DeptName != nil {
		cols["dept_name"] = *req.DeptName
	}
	if req.ParentID != nil {
		pid := int64(*req.ParentID)
		if pid == 0 {
			// 显式移至根：parent_id=0 + parent_name 清 NULL（与 Create 语义对齐，
			// 源 MP null-skip 残留旧值的有意修正，同菜单域）
			cols["parent_id"] = 0
			cols["parent_name"] = nil
		} else {
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
		return nil, err
	}
	return s.Get(ctx, id)
}

// Get 部门详情：按主键查询后 PO→DTO（未命中 404/10300）。
func (s *DeptService) Get(ctx context.Context, id int64) (*dto.DeptResp, error) {
	d, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return dto.ToDeptResp(d), nil
}

// List 部门全量列表（无分页——树与源 list 语义；ORDER BY sort ASC 由 repo 承担）。
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
// 孤儿（parentId 为 nil/0/指向不存在节点）均提升为根——DEPT-02 验收明文
// （源服务层 buildTree 语义，非实体死代码的孤儿丢弃语义）。
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

// Delete 删除部门：直接物理删。与源一致**不加子部门/用户引用检查**（源缺口按源
// 保留：DeptApplicationService.removeById 直接删）。
func (s *DeptService) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

// resolveParent 父校验（源 resolveParentName 逐字语义）：父不存在或父 dept_name
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
		// 源：parent == null || parent.getDeptName() == null 同样拒绝
		return nil, errcode.New(errcode.CodeInvalidParam, fmt.Sprintf("上级部门不存在: %d", pid))
	}
	return parent, nil
}
