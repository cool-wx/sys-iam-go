// Package handler 排班域 HTTP handler（薄层三段式；List 全量数组直出——
// 源 list 无分页，直译，勿统一为分页）。
package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/cool-wx/sys-iam-go/internal/model/dto"
	"github.com/cool-wx/sys-iam-go/internal/service"
	"github.com/cool-wx/sys-iam-go/pkg/resp"
)

// ShiftHandler 排班域 HTTP handler。
type ShiftHandler struct {
	svc *service.ShiftService
}

// NewShiftHandler 构造。
func NewShiftHandler(svc *service.ShiftService) *ShiftHandler {
	return &ShiftHandler{svc: svc}
}

// Create 处理 POST /api/v1/shifts：绑定校验 → service → envelope 出口。
func (h *ShiftHandler) Create(c *gin.Context) {
	var req dto.ShiftCreateReq
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

// Update 处理 PUT /api/v1/shifts/:id：解析 :id → 绑定指针 DTO → service → envelope。
func (h *ShiftHandler) Update(c *gin.Context) {
	id, ok := parseID(c, "无效的排班 ID")
	if !ok {
		return
	}
	var req dto.ShiftUpdateReq
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

// Get 处理 GET /api/v1/shifts/:id：解析 :id → service → envelope。
func (h *ShiftHandler) Get(c *gin.Context) {
	id, ok := parseID(c, "无效的排班 ID")
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

// List 处理 GET /api/v1/shifts：全量数组直出——源 list 无分页直译，
// 勿统一为分页（data 直接是数组，无 items/total/page/size envelope）。
func (h *ShiftHandler) List(c *gin.Context) {
	data, err := h.svc.List(c.Request.Context())
	if err != nil {
		resp.Fail(c, err)
		return
	}
	resp.OK(c, data)
}

// Delete 处理 DELETE /api/v1/shifts/:id：解析 :id → service（物理删）→ envelope。
func (h *ShiftHandler) Delete(c *gin.Context) {
	id, ok := parseID(c, "无效的排班 ID")
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), id); err != nil {
		resp.Fail(c, err)
		return
	}
	resp.OK(c, nil)
}
