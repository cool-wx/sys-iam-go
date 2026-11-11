// Package handler 字典类型域 HTTP handler（薄层三段式：bind/parse → svc → envelope；
// 路径参数统一走 parseID 通用解析）。
package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/cool-wx/sys-iam-go/internal/model/dto"
	"github.com/cool-wx/sys-iam-go/internal/service"
	"github.com/cool-wx/sys-iam-go/pkg/paging"
	"github.com/cool-wx/sys-iam-go/pkg/resp"
)

// DictTypeHandler 字典类型域 HTTP handler。
type DictTypeHandler struct {
	svc *service.DictService
}

// NewDictTypeHandler 构造。
func NewDictTypeHandler(svc *service.DictService) *DictTypeHandler {
	return &DictTypeHandler{svc: svc}
}

// Create 处理 POST /api/v1/dict-types：绑定校验 → service → envelope 出口。
func (h *DictTypeHandler) Create(c *gin.Context) {
	var req dto.DictTypeCreateReq
	if !bindJSON(c, &req) {
		return
	}
	data, err := h.svc.CreateDictType(c.Request.Context(), &req)
	if err != nil {
		resp.Fail(c, err)
		return
	}
	resp.OK(c, data)
}

// Update 处理 PUT /api/v1/dict-types/:id：解析 :id → 绑定指针 DTO → service → envelope。
func (h *DictTypeHandler) Update(c *gin.Context) {
	id, ok := parseID(c, "无效的字典类型 ID")
	if !ok {
		return
	}
	var req dto.DictTypeUpdateReq
	if !bindJSON(c, &req) {
		return
	}
	data, err := h.svc.UpdateDictType(c.Request.Context(), id, &req)
	if err != nil {
		resp.Fail(c, err)
		return
	}
	resp.OK(c, data)
}

// Get 处理 GET /api/v1/dict-types/:id：解析 :id → service → envelope。
func (h *DictTypeHandler) Get(c *gin.Context) {
	id, ok := parseID(c, "无效的字典类型 ID")
	if !ok {
		return
	}
	data, err := h.svc.GetDictType(c.Request.Context(), id)
	if err != nil {
		resp.Fail(c, err)
		return
	}
	resp.OK(c, data)
}

// List 处理 GET /api/v1/dict-types：分页列表（paging.Parse 1/15 钳位）→ envelope。
func (h *DictTypeHandler) List(c *gin.Context) {
	page, size := paging.Parse(c.Query("page"), c.Query("size"))
	data, err := h.svc.ListDictTypes(c.Request.Context(), page, size)
	if err != nil {
		resp.Fail(c, err)
		return
	}
	resp.OK(c, data)
}

// Delete 处理 DELETE /api/v1/dict-types/:id：解析 :id → service（物理删）→ envelope。
func (h *DictTypeHandler) Delete(c *gin.Context) {
	id, ok := parseID(c, "无效的字典类型 ID")
	if !ok {
		return
	}
	if err := h.svc.DeleteDictType(c.Request.Context(), id); err != nil {
		resp.Fail(c, err)
		return
	}
	resp.OK(c, nil)
}
