// Package handler API Key 域 HTTP handler（薄层三段式）。
package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/cool-wx/sys-iam-go/internal/model/dto"
	"github.com/cool-wx/sys-iam-go/internal/service"
	"github.com/cool-wx/sys-iam-go/pkg/paging"
	"github.com/cool-wx/sys-iam-go/pkg/resp"
)

// ApiKeyHandler API Key 域 HTTP handler。
type ApiKeyHandler struct {
	svc *service.ApiKeyService
}

// NewApiKeyHandler 构造。
func NewApiKeyHandler(svc *service.ApiKeyService) *ApiKeyHandler {
	return &ApiKeyHandler{svc: svc}
}

// Create 处理 POST /api/v1/api-keys：绑定校验 → service → envelope。响应 data
// 含**明文 apiKey**——一次性展示语义（客户端须即时保存，此后管理面 list/get
// 虽可见但无再次"生成/重置"路径）。
func (h *ApiKeyHandler) Create(c *gin.Context) {
	var req dto.ApiKeyCreateReq
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

// List 处理 GET /api/v1/api-keys：query page/size 经 paging.Parse 钳位 → service
// → envelope（分页，管理面可见明文 key）。
func (h *ApiKeyHandler) List(c *gin.Context) {
	page, size := paging.Parse(c.Query("page"), c.Query("size"))
	data, err := h.svc.List(c.Request.Context(), page, size)
	if err != nil {
		resp.Fail(c, err)
		return
	}
	resp.OK(c, data)
}

// Get 处理 GET /api/v1/api-keys/:id：解析 :id → service → envelope。
func (h *ApiKeyHandler) Get(c *gin.Context) {
	id, ok := parseID(c, "无效的 API Key ID")
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

// UpdateStatus 处理 PUT /api/v1/api-keys/:id/status：仅 status 单列语义
// （停用立即生效）。
func (h *ApiKeyHandler) UpdateStatus(c *gin.Context) {
	id, ok := parseID(c, "无效的 API Key ID")
	if !ok {
		return
	}
	var req dto.ApiKeyUpdateStatusReq
	if !bindJSON(c, &req) {
		return
	}
	if err := h.svc.UpdateStatus(c.Request.Context(), id, *req.Status); err != nil {
		resp.Fail(c, err)
		return
	}
	resp.OK(c, nil)
}

// Delete 处理 DELETE /api/v1/api-keys/:id：物理删。
func (h *ApiKeyHandler) Delete(c *gin.Context) {
	id, ok := parseID(c, "无效的 API Key ID")
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), id); err != nil {
		resp.Fail(c, err)
		return
	}
	resp.OK(c, nil)
}
