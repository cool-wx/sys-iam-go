// Package handler 部门域 HTTP handler（薄层三段式，与菜单域同构）。
package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/cool-wx/sys-iam-go/internal/model/dto"
	"github.com/cool-wx/sys-iam-go/internal/service"
	"github.com/cool-wx/sys-iam-go/pkg/resp"
)

// DeptHandler 部门域 HTTP handler。
type DeptHandler struct {
	svc *service.DeptService
}

// NewDeptHandler 构造。
func NewDeptHandler(svc *service.DeptService) *DeptHandler {
	return &DeptHandler{svc: svc}
}

// Create 处理 POST /api/v1/depts：绑定校验 → service → envelope 出口。
func (h *DeptHandler) Create(c *gin.Context) {
	var req dto.DeptCreateReq
	if !bindJSON(c, &req) {
		return
	}
	data, err := h.svc.Create(c.Request.Context(), &req)
	if err != nil {
		resp.Fail(c, err)
		return
	}
	resp.OK(c, data)
}

// Update 处理 PUT /api/v1/depts/:id：解析 :id → 绑定指针 DTO → service → envelope。
func (h *DeptHandler) Update(c *gin.Context) {
	id, ok := parseID(c, "无效的部门 ID")
	if !ok {
		return
	}
	var req dto.DeptUpdateReq
	if !bindJSON(c, &req) {
		return
	}
	data, err := h.svc.Update(c.Request.Context(), id, &req)
	if err != nil {
		resp.Fail(c, err)
		return
	}
	resp.OK(c, data)
}

// Get 处理 GET /api/v1/depts/:id：解析 :id → service → envelope。
func (h *DeptHandler) Get(c *gin.Context) {
	id, ok := parseID(c, "无效的部门 ID")
	if !ok {
		return
	}
	data, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		resp.Fail(c, err)
		return
	}
	resp.OK(c, data)
}

// List 处理 GET /api/v1/depts：全量列表（无分页——树查询需要完整数据集）→ envelope。
func (h *DeptHandler) List(c *gin.Context) {
	data, err := h.svc.List(c.Request.Context())
	if err != nil {
		resp.Fail(c, err)
		return
	}
	resp.OK(c, data)
}

// Tree 处理 GET /api/v1/depts/tree：父子层级树（孤儿提升为根）→ envelope。
func (h *DeptHandler) Tree(c *gin.Context) {
	data, err := h.svc.Tree(c.Request.Context())
	if err != nil {
		resp.Fail(c, err)
		return
	}
	resp.OK(c, data)
}

// Delete 处理 DELETE /api/v1/depts/:id：解析 :id → service（物理删）→ envelope。
func (h *DeptHandler) Delete(c *gin.Context) {
	id, ok := parseID(c, "无效的部门 ID")
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), id); err != nil {
		resp.Fail(c, err)
		return
	}
	resp.OK(c, nil)
}
