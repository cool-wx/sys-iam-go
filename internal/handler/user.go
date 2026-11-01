// Package handler 定义 HTTP 协议层：绑定/校验/调 service/写 envelope（薄，按域一文件）。
package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/cool-wx/sys-iam-go/internal/model/dto"
	"github.com/cool-wx/sys-iam-go/internal/service"
	"github.com/cool-wx/sys-iam-go/pkg/errcode"
	"github.com/cool-wx/sys-iam-go/pkg/paging"
	"github.com/cool-wx/sys-iam-go/pkg/resp"
)

// UserHandler 用户域 HTTP handler。
type UserHandler struct {
	svc *service.UserService
}

// NewUserHandler 构造。
func NewUserHandler(svc *service.UserService) *UserHandler {
	return &UserHandler{svc: svc}
}

// bindJSON 绑定 JSON 请求体。请求体超限（BodyLimit 中间件的 MaxBytesReader，
// WR-02）时收敛为固定文案 10401 envelope——不回显底层错误原文（T-1-01 纪律）；
// 其余校验失败维持既有文案。返回 false 表示响应已写出，调用方直接 return。
func bindJSON(c *gin.Context, obj any) bool {
	if err := c.ShouldBindJSON(obj); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			// 文案由实际触发的截断层 limit 生成（MaxBytesError.Limit，IN-02）——
			// multipart Content-Type 打 JSON 路由超全局 multipart 兜底（CR-01，
			// 20MB）时文案报 20 而非按 MaxBodyMB 固定生成的 1，与实际上限永不漂移；
			// 各 BodyLimit 层上限均为整 MB（<<20 派生），右移整除安全。
			resp.Fail(c, errcode.New(errcode.CodeValidation,
				fmt.Sprintf("请求体超过 %dMB 上限", tooLarge.Limit>>20)))
			return false
		}
		resp.Fail(c, errcode.New(errcode.CodeValidation, "参数校验失败: "+err.Error()))
		return false
	}
	return true
}

// Create 处理 POST /api/v1/users：绑定校验 → service → envelope 出口。
func (h *UserHandler) Create(c *gin.Context) {
	var req dto.UserCreateReq
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

// Update 处理 PUT /api/v1/users/:id：解析 :id → 绑定指针 DTO → service → envelope。
func (h *UserHandler) Update(c *gin.Context) {
	id, ok := parseUserID(c)
	if !ok {
		return
	}
	var req dto.UpdateUserReq
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

// Get 处理 GET /api/v1/users/:id：解析 :id → service → envelope。
func (h *UserHandler) Get(c *gin.Context) {
	id, ok := parseUserID(c)
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

// List 处理 GET /api/v1/users：query page/size 经 paging.Parse 钳位 → service → envelope。
func (h *UserHandler) List(c *gin.Context) {
	page, size := paging.Parse(c.Query("page"), c.Query("size"))
	data, err := h.svc.List(c.Request.Context(), page, size)
	if err != nil {
		resp.Fail(c, err)
		return
	}
	resp.OK(c, data)
}

// parseUserID 解析路径参数 :id（十进制 int64）；失败时写出 400/10400 并返回 false。
func parseUserID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		resp.Fail(c, errcode.New(errcode.CodeInvalidParam, "无效的用户 ID"))
		return 0, false
	}
	return id, true
}

// GetRoles 处理 GET /api/v1/users/:id/roles：返回用户当前角色列表（USER-04）。
func (h *UserHandler) GetRoles(c *gin.Context) {
	id, ok := parseUserID(c)
	if !ok {
		return
	}
	data, err := h.svc.GetRoles(c.Request.Context(), id)
	if err != nil {
		resp.Fail(c, err)
		return
	}
	resp.OK(c, data)
}

// AssignRoles 处理 PUT /api/v1/users/:id/roles：全量替换语义（USER-04/D-12，
// 空数组=全回收）。
func (h *UserHandler) AssignRoles(c *gin.Context) {
	id, ok := parseUserID(c)
	if !ok {
		return
	}
	var req dto.AssignRolesReq
	if !bindJSON(c, &req) {
		return
	}
	if err := h.svc.AssignRoles(c.Request.Context(), id, req.RoleIDs); err != nil {
		resp.Fail(c, err)
		return
	}
	resp.OK(c, nil)
}

// UpdateStatus 处理 PUT /api/v1/users/:id/status：启/停用；停用即踢存量会话
// （D-09 安全增强）。
func (h *UserHandler) UpdateStatus(c *gin.Context) {
	id, ok := parseUserID(c)
	if !ok {
		return
	}
	var req dto.UpdateStatusReq
	if !bindJSON(c, &req) {
		return
	}
	if err := h.svc.UpdateStatus(c.Request.Context(), id, *req.Status); err != nil {
		resp.Fail(c, err)
		return
	}
	resp.OK(c, nil)
}

// ResetPassword 处理 PUT /api/v1/users/:id/reset-password：固定 123456 + 踢存量
// 会话；无请求体（源 resetPwd 无旧密码校验，USER-03）。
func (h *UserHandler) ResetPassword(c *gin.Context) {
	id, ok := parseUserID(c)
	if !ok {
		return
	}
	if err := h.svc.ResetPassword(c.Request.Context(), id); err != nil {
		resp.Fail(c, err)
		return
	}
	resp.OK(c, nil)
}

// Delete 处理 DELETE /api/v1/users/:id：builtin 守卫（409/10601 零删除调用）+
// 先删 sys_user_role 关联再删用户（事务，源 removeById 顺序，USER-07）。
func (h *UserHandler) Delete(c *gin.Context) {
	id, ok := parseUserID(c)
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), id); err != nil {
		resp.Fail(c, err)
		return
	}
	resp.OK(c, nil)
}
