// Package handler 定义 HTTP 协议层：绑定/校验/调 service/写 envelope（薄，按域一文件）。
package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/cool-wx/sys-iam-go/internal/model/dto"
	"github.com/cool-wx/sys-iam-go/internal/service"
	"github.com/cool-wx/sys-iam-go/pkg/errcode"
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

// Create 处理 POST /api/v1/users：绑定校验 → service → envelope 出口。
func (h *UserHandler) Create(c *gin.Context) {
	var req dto.UserCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Fail(c, errcode.New(errcode.CodeValidation, "参数校验失败: "+err.Error()))
		return
	}
	data, err := h.svc.Create(c.Request.Context(), &req)
	if err != nil {
		resp.Fail(c, err)
		return
	}
	resp.OK(c, data)
}
