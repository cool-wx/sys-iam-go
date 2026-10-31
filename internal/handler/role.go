// Package handler 角色域 HTTP handler（薄层三段式）+ 角色菜单分配子资源。
package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/cool-wx/sys-iam-go/internal/model/dto"
	"github.com/cool-wx/sys-iam-go/internal/service"
	"github.com/cool-wx/sys-iam-go/pkg/paging"
	"github.com/cool-wx/sys-iam-go/pkg/resp"
)

// RoleHandler 角色域 HTTP handler。
type RoleHandler struct {
	svc *service.RoleService
}

// NewRoleHandler 构造。
func NewRoleHandler(svc *service.RoleService) *RoleHandler {
	return &RoleHandler{svc: svc}
}

// Create 处理 POST /api/v1/roles：绑定校验 → service → envelope 出口。
func (h *RoleHandler) Create(c *gin.Context) {
	var req dto.RoleCreateReq
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

// Update 处理 PUT /api/v1/roles/:id：解析 :id → 绑定指针 DTO → service → envelope。
func (h *RoleHandler) Update(c *gin.Context) {
	id, ok := parseID(c, "无效的角色 ID")
	if !ok {
		return
	}
	var req dto.RoleUpdateReq
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

// Get 处理 GET /api/v1/roles/:id：解析 :id → service → envelope。
func (h *RoleHandler) Get(c *gin.Context) {
	id, ok := parseID(c, "无效的角色 ID")
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

// List 处理 GET /api/v1/roles：query page/size 经 paging.Parse 钳位 + roleName
// 条件（trim LIKE + id ASC，源特例）→ service → envelope。
func (h *RoleHandler) List(c *gin.Context) {
	page, size := paging.Parse(c.Query("page"), c.Query("size"))
	data, err := h.svc.List(c.Request.Context(), page, size, c.Query("roleName"))
	if err != nil {
		resp.Fail(c, err)
		return
	}
	resp.OK(c, data)
}

// Delete 处理 DELETE /api/v1/roles/:id：解析 :id → service（builtin 守卫 + 三连删
// 事务）→ envelope。
func (h *RoleHandler) Delete(c *gin.Context) {
	id, ok := parseID(c, "无效的角色 ID")
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), id); err != nil {
		resp.Fail(c, err)
		return
	}
	resp.OK(c, nil)
}

// GetMenus 处理 GET /api/v1/roles/:id/menus：返回角色已分配 menuId 列表。
func (h *RoleHandler) GetMenus(c *gin.Context) {
	id, ok := parseID(c, "无效的角色 ID")
	if !ok {
		return
	}
	data, err := h.svc.GetMenuIDs(c.Request.Context(), id)
	if err != nil {
		resp.Fail(c, err)
		return
	}
	resp.OK(c, data)
}

// AssignMenus 处理 PUT /api/v1/roles/:id/menus：全量替换语义（删旧插新同事务）。
func (h *RoleHandler) AssignMenus(c *gin.Context) {
	id, ok := parseID(c, "无效的角色 ID")
	if !ok {
		return
	}
	var req dto.AssignMenusReq
	if !bindJSON(c, &req) {
		return
	}
	if err := h.svc.AssignMenus(c.Request.Context(), id, req.MenuIDs); err != nil {
		resp.Fail(c, err)
		return
	}
	resp.OK(c, nil)
}
