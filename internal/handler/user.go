// Package handler 定义 HTTP 协议层：绑定/校验/调 service/写 envelope（薄，按域一文件）。
package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/cool-wx/sys-iam-go/internal/middleware"
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
			// 文案由 middleware.MaxBodyMB 生成（单一事实源，IN-10）——上限调整时
			// 文案随之更新，不会过期失真。
			resp.Fail(c, errcode.New(errcode.CodeValidation,
				fmt.Sprintf("请求体超过 %dMB 上限", middleware.MaxBodyMB)))
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
