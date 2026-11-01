// Package handler 系统配置域 HTTP handler（薄层三段式；GetByKey 响应 data 为
// configValue 字符串本体——源 getByKey 返回 String，非对象包装）。
package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/cool-wx/sys-iam-go/internal/model/dto"
	"github.com/cool-wx/sys-iam-go/internal/service"
	"github.com/cool-wx/sys-iam-go/pkg/paging"
	"github.com/cool-wx/sys-iam-go/pkg/resp"
)

// ConfigHandler 系统配置域 HTTP handler。
type ConfigHandler struct {
	svc *service.ConfigService
}

// NewConfigHandler 构造。
func NewConfigHandler(svc *service.ConfigService) *ConfigHandler {
	return &ConfigHandler{svc: svc}
}

// Create 处理 POST /api/v1/configs：绑定校验 → service → envelope 出口。
func (h *ConfigHandler) Create(c *gin.Context) {
	var req dto.ConfigCreateReq
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

// Update 处理 PUT /api/v1/configs/:id：解析 :id → 绑定指针 DTO → service → envelope。
func (h *ConfigHandler) Update(c *gin.Context) {
	id, ok := parseID(c, "无效的配置 ID")
	if !ok {
		return
	}
	var req dto.ConfigUpdateReq
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

// Get 处理 GET /api/v1/configs/:id：解析 :id → service → envelope。
func (h *ConfigHandler) Get(c *gin.Context) {
	id, ok := parseID(c, "无效的配置 ID")
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

// GetByKey 处理 GET /api/v1/configs/key/:key：响应 data 为 configValue 字符串
// 本体（源 getByKey 返回 String 语义，CONF-01——非 ConfigResp 对象包装）。
func (h *ConfigHandler) GetByKey(c *gin.Context) {
	key := c.Param("key")
	po, err := h.svc.GetByKey(c.Request.Context(), key)
	if err != nil {
		resp.Fail(c, err)
		return
	}
	resp.OK(c, po.ConfigValue)
}

// List 处理 GET /api/v1/configs：分页列表（paging.Parse 1/15 钳位）→ envelope。
func (h *ConfigHandler) List(c *gin.Context) {
	page, size := paging.Parse(c.Query("page"), c.Query("size"))
	data, err := h.svc.List(c.Request.Context(), page, size)
	if err != nil {
		resp.Fail(c, err)
		return
	}
	resp.OK(c, data)
}

// Delete 处理 DELETE /api/v1/configs/:id：解析 :id → service（物理删）→ envelope。
func (h *ConfigHandler) Delete(c *gin.Context) {
	id, ok := parseID(c, "无效的配置 ID")
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), id); err != nil {
		resp.Fail(c, err)
		return
	}
	resp.OK(c, nil)
}
