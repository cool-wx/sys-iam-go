// Package handler 菜单域 HTTP handler（薄层三段式：bind/parse → svc → envelope）。
package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/cool-wx/sys-iam-go/internal/model/dto"
	"github.com/cool-wx/sys-iam-go/internal/service"
	"github.com/cool-wx/sys-iam-go/pkg/errcode"
	"github.com/cool-wx/sys-iam-go/pkg/resp"
)

// MenuHandler 菜单域 HTTP handler。
type MenuHandler struct {
	svc *service.MenuService
}

// NewMenuHandler 构造。
func NewMenuHandler(svc *service.MenuService) *MenuHandler {
	return &MenuHandler{svc: svc}
}

// parseID 解析路径参数 :id（十进制 int64）；失败时写出 400/10400 并返回 false。
// label 按域文案（"无效的菜单 ID"/"无效的部门 ID"/"无效的角色 ID"）——泛化自
// parseUserID 样板，新建域 handler 统一走本函数。
func parseID(c *gin.Context, label string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		resp.Fail(c, errcode.New(errcode.CodeInvalidParam, label))
		return 0, false
	}
	return id, true
}

// Create 处理 POST /api/v1/menus：绑定校验 → service → envelope 出口。
func (h *MenuHandler) Create(c *gin.Context) {
	var req dto.MenuCreateReq
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

// Update 处理 PUT /api/v1/menus/:id：解析 :id → 绑定指针 DTO → service → envelope。
func (h *MenuHandler) Update(c *gin.Context) {
	id, ok := parseID(c, "无效的菜单 ID")
	if !ok {
		return
	}
	var req dto.MenuUpdateReq
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

// Get 处理 GET /api/v1/menus/:id：解析 :id → service → envelope。
func (h *MenuHandler) Get(c *gin.Context) {
	id, ok := parseID(c, "无效的菜单 ID")
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

// List 处理 GET /api/v1/menus：全量列表（无分页——树与源 list 语义）→ envelope。
func (h *MenuHandler) List(c *gin.Context) {
	data, err := h.svc.List(c.Request.Context())
	if err != nil {
		resp.Fail(c, err)
		return
	}
	resp.OK(c, data)
}

// Tree 处理 GET /api/v1/menus/tree：父子层级树（孤儿提升为根）→ envelope。
func (h *MenuHandler) Tree(c *gin.Context) {
	data, err := h.svc.Tree(c.Request.Context())
	if err != nil {
		resp.Fail(c, err)
		return
	}
	resp.OK(c, data)
}

// Delete 处理 DELETE /api/v1/menus/:id：解析 :id → service（物理删）→ envelope。
func (h *MenuHandler) Delete(c *gin.Context) {
	id, ok := parseID(c, "无效的菜单 ID")
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), id); err != nil {
		resp.Fail(c, err)
		return
	}
	resp.OK(c, nil)
}
