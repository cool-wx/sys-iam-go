// Package service 文件分类域业务逻辑（与部门域同构）：父校验 + parentName 服务端
// 冗余 + 树查询（FILE-04；树构建复用 internal/tree 泛型构建器，孤儿提升语义与
// dept/menu 一致——以源服务层 buildTree 为准，勿仿实体静态 buildTree 的不提升版本，
// 04-RESEARCH 重点 1 差异警示）。
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

// FileCategoryService 文件分类域服务。
type FileCategoryService struct {
	repo *repository.FileCategoryRepo
}

// NewFileCategoryService 构造。
func NewFileCategoryService(repo *repository.FileCategoryRepo) *FileCategoryService {
	return &FileCategoryService{repo: repo}
}

// Create 创建文件分类：DTO→PO（服务端默认 status=1 正常——对齐 DDL default 与源
// MP null-skip 语义）；parentId 非 nil 且非 0 时校验父存在并冗余 parentName
// （源 FileCategoryApplicationService add 冗余语义；父缺失译为 404/10300
// 「上级分类不存在: {pid}」源消息原文）。
func (s *FileCategoryService) Create(ctx context.Context, req *dto.FileCategoryCreateReq) (*dto.FileCategoryResp, error) {
	fc := &po.FileCategoryPO{
		CategoryName: req.CategoryName,
		CategoryType: req.CategoryType,
		Sort:         req.Sort,
		Icon:         req.Icon,
		Remark:       req.Remark,
	}
	status := int8(1)
	fc.Status = &status
	if req.ParentID != nil && *req.ParentID != 0 {
		pid := int64(*req.ParentID)
		parent, err := s.resolveParent(ctx, pid)
		if err != nil {
			return nil, err
		}
		fc.ParentID = &pid
		fc.ParentName = parent.CategoryName // 服务端查库冗余，不信请求体
	}
	if err := s.repo.Create(ctx, fc); err != nil {
		return nil, err
	}
	return dto.ToFileCategoryResp(fc), nil
}

// Update 更新文件分类（部分更新，ENG-05）：白名单 categoryName/parentId/
// categoryType/sort/icon/status/remark；parentId 显式传入时联动父校验 + parentName
// 重冗余（源 edit 同构语义）。落库后回读返回最新契约；审计由 repo.Patch 显式并入
// （ENG-02）。WR-04 成环拦截与部门域同构（自引用/挂到自身子孙均拒绝）。
func (s *FileCategoryService) Update(ctx context.Context, id int64, req *dto.FileCategoryUpdateReq) (*dto.FileCategoryResp, error) {
	cols := map[string]any{}
	if req.CategoryName != nil {
		cols["category_name"] = *req.CategoryName
	}
	if req.ParentID != nil {
		pid := int64(*req.ParentID)
		if pid == 0 {
			// 显式移至根：parent_id 落 NULL + parent_name 清 NULL——根 = parent_id
			// NULL 成为唯一形态（WR-07 方向 A，与 Create 根同形态；源自身 Create
			// null / Update 0 不对称，此处为 Go 内一致性修正、显式偏离源）。存量
			// parent_id=0 行不迁移：tree.Build/ensureNotDescendant/resolveParent
			// 对 nil/0 双形态兼容，读侧零回归。GORM Updates(map) 显式 nil 写 NULL
			//（与同分支 parent_name 清 NULL 同款）。
			cols["parent_id"] = nil
			cols["parent_name"] = nil
		} else {
			if pid == id {
				return nil, errcode.New(errcode.CodeInvalidParam, "上级分类不能是自己")
			}
			if err := s.ensureNotDescendant(ctx, pid, id); err != nil {
				return nil, err
			}
			parent, err := s.resolveParent(ctx, pid)
			if err != nil {
				return nil, err
			}
			cols["parent_id"] = pid
			cols["parent_name"] = *parent.CategoryName
		}
	}
	if req.CategoryType != nil {
		cols["category_type"] = *req.CategoryType
	}
	if req.Sort != nil {
		cols["sort"] = *req.Sort
	}
	if req.Icon != nil {
		cols["icon"] = *req.Icon
	}
	if req.Status != nil {
		cols["status"] = int8(*req.Status)
	}
	if req.Remark != nil {
		cols["remark"] = *req.Remark
	}
	if err := s.repo.Patch(ctx, id, cols); err != nil {
		if errors.Is(err, errcode.ErrNotFound) {
			return nil, errcode.New(errcode.CodeNotFound, fmt.Sprintf("文件分类不存在: %d", id))
		}
		return nil, err
	}
	return s.Get(ctx, id)
}

// Get 文件分类详情：未命中译为 404/10300「文件分类不存在: {id}」。
func (s *FileCategoryService) Get(ctx context.Context, id int64) (*dto.FileCategoryResp, error) {
	fc, err := s.repo.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, errcode.ErrNotFound) {
			return nil, errcode.New(errcode.CodeNotFound, fmt.Sprintf("文件分类不存在: %d", id))
		}
		return nil, err
	}
	return dto.ToFileCategoryResp(fc), nil
}

// Tree 文件分类树：List 全量 → tree.Build → []FileCategoryTreeResp。
// 孤儿（parentId 为 nil/0/指向不存在节点）均提升为根——孤儿提升语义与 dept 一致
// （源服务层 buildTree 语义；源 Controller 调用路径即 service 实现）。
func (s *FileCategoryService) Tree(ctx context.Context) ([]*dto.FileCategoryTreeResp, error) {
	items, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	respItems := make([]dto.FileCategoryTreeResp, 0, len(items))
	for i := range items {
		respItems = append(respItems, *dto.ToFileCategoryTreeResp(&items[i]))
	}
	roots := tree.Build(respItems,
		func(fc dto.FileCategoryTreeResp) int64 { return int64(fc.ID) },
		func(fc dto.FileCategoryTreeResp) *int64 {
			if fc.ParentID == nil {
				return nil
			}
			v := int64(*fc.ParentID)
			return &v
		},
		func(node *dto.FileCategoryTreeResp, children []dto.FileCategoryTreeResp) {
			node.Children = make([]*dto.FileCategoryTreeResp, 0, len(children))
			for i := range children {
				c := children[i]
				node.Children = append(node.Children, &c)
			}
		},
	)
	out := make([]*dto.FileCategoryTreeResp, 0, len(roots))
	for i := range roots {
		r := roots[i]
		out = append(out, &r)
	}
	return out, nil
}

// Delete 删除文件分类：直接物理删。与源一致**不加子分类/文件引用检查**（源缺口
// 按源保留：FileCategoryApplicationService.removeById 直接删）。
func (s *FileCategoryService) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

// ensureNotDescendant 成环拦截（WR-04，与部门域同构）：把 id 挂到 pid 下前，沿
// pid 的父链向上上溯（visited 集防存量环数据死循环 + 深度上限冗余兜底）——若
// 途中遇到 id，说明 pid 位于 id 的子链上，挂接将成环，报 400。父链中断（节点
// 不存在/到根）即无环。
func (s *FileCategoryService) ensureNotDescendant(ctx context.Context, pid, id int64) error {
	visited := make(map[int64]bool)
	const maxDepth = 100
	cur := pid
	for depth := 0; depth < maxDepth && !visited[cur]; depth++ {
		if cur == id {
			return errcode.New(errcode.CodeInvalidParam, fmt.Sprintf("上级分类不能是当前分类的子孙节点: %d", pid))
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

// resolveParent 父校验（源 resolveParentName 语义；父缺失译为 404/10300 源消息
// 原文——与部门域 400 不同，04-02 PLAN Task 2 action 5 显式拍板 CodeNotFound）：
// 父不存在或父 category_name 为 NULL 均报「上级分类不存在: {pid}」；parentName
// 只信服务端查库值。
func (s *FileCategoryService) resolveParent(ctx context.Context, pid int64) (*po.FileCategoryPO, error) {
	parent, err := s.repo.FindByID(ctx, pid)
	if err != nil {
		if errors.Is(err, errcode.ErrNotFound) {
			return nil, errcode.New(errcode.CodeNotFound, fmt.Sprintf("上级分类不存在: %d", pid))
		}
		return nil, err
	}
	if parent.CategoryName == nil {
		// 源：parent == null || parent.getCategoryName() == null 同样拒绝
		return nil, errcode.New(errcode.CodeNotFound, fmt.Sprintf("上级分类不存在: %d", pid))
	}
	return parent, nil
}
