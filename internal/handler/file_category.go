// Package handler 文件分类域 HTTP handler（五方法照 dept handler 同构；树端点
// 静态段 tree 与 :id 参数段共存，静态优先）。
package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/cool-wx/sys-iam-go/internal/model/dto"
	"github.com/cool-wx/sys-iam-go/internal/service"
	"github.com/cool-wx/sys-iam-go/pkg/resp"
)

// FileCategoryHandler 文件分类域 HTTP handler。
type FileCategoryHandler struct {
	svc *service.FileCategoryService
}

// NewFileCategoryHandler 构造。
func NewFileCategoryHandler(svc *service.FileCategoryService) *FileCategoryHandler {
	return &FileCategoryHandler{svc: svc}
}

// Create 处理 POST /api/v1/file-categories：绑定校验 → service → envelope 出口。
func (h *FileCategoryHandler) Create(c *gin.Context) {
	var req dto.FileCategoryCreateReq
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

// Update 处理 PUT /api/v1/file-categories/:id：解析 :id → 绑定指针 DTO → service → envelope。
func (h *FileCategoryHandler) Update(c *gin.Context) {
	id, ok := parseID(c, "无效的文件分类 ID")
	if !ok {
		return
	}
	var req dto.FileCategoryUpdateReq
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

// Get 处理 GET /api/v1/file-categories/:id：解析 :id → service → envelope。
func (h *FileCategoryHandler) Get(c *gin.Context) {
	id, ok := parseID(c, "无效的文件分类 ID")
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

// Tree 处理 GET /api/v1/file-categories/tree：父子层级树（孤儿提升为根）→ envelope。
func (h *FileCategoryHandler) Tree(c *gin.Context) {
	data, err := h.svc.Tree(c.Request.Context())
	if err != nil {
		resp.Fail(c, err)
		return
	}
	resp.OK(c, data)
}

// Delete 处理 DELETE /api/v1/file-categories/:id：解析 :id → service（物理删）→ envelope。
func (h *FileCategoryHandler) Delete(c *gin.Context) {
	id, ok := parseID(c, "无效的文件分类 ID")
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), id); err != nil {
		resp.Fail(c, err)
		return
	}
	resp.OK(c, nil)
}
