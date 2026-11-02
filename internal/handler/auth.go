package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/cool-wx/sys-iam-go/internal/middleware"
	"github.com/cool-wx/sys-iam-go/internal/model/dto"
	"github.com/cool-wx/sys-iam-go/internal/service"
	"github.com/cool-wx/sys-iam-go/pkg/errcode"
	"github.com/cool-wx/sys-iam-go/pkg/resp"
)

// AuthHandler 认证域 HTTP handler（login/logout；apikey-login/kickout 随 Task 2 扩展）。
type AuthHandler struct {
	svc *service.AuthService
}

// NewAuthHandler 构造。
func NewAuthHandler(svc *service.AuthService) *AuthHandler {
	return &AuthHandler{svc: svc}
}

// Login 处理 POST /api/v1/auth/login：绑定校验 → service → envelope 出口。
func (h *AuthHandler) Login(c *gin.Context) {
	var req dto.LoginReq
	if !bindJSON(c, &req) {
		return
	}
	data, err := h.svc.Login(c.Request.Context(), &req)
	if err != nil {
		resp.Fail(c, err)
		return
	}
	resp.OK(c, data)
}

// Logout 处理 POST /api/v1/auth/logout：token 从 Authorization Bearer 提取（与认证
// 中间件同一解析规则）；本端点挂在 pri 组（调用方必然已持有效 token），头缺失/前缀
// 不符按未登录收敛（10100/401）。
func (h *AuthHandler) Logout(c *gin.Context) {
	token, ok := middleware.BearerToken(c.GetHeader("Authorization"))
	if !ok {
		resp.Fail(c, errcode.ErrUnauthenticated)
		return
	}
	if err := h.svc.Logout(c.Request.Context(), token); err != nil {
		resp.Fail(c, err)
		return
	}
	resp.OK(c, nil)
}

// ApikeyLogin 处理 POST /api/v1/auth/apikey-login（pub 白名单第四路径）：
// 绑定校验 → service → envelope 出口。
func (h *AuthHandler) ApikeyLogin(c *gin.Context) {
	var req dto.ApiKeyLoginReq
	if !bindJSON(c, &req) {
		return
	}
	data, err := h.svc.ApikeyLogin(c.Request.Context(), &req)
	if err != nil {
		resp.Fail(c, err)
		return
	}
	resp.OK(c, data)
}

// Kickout 处理 POST /api/v1/auth/kickout（pri 组）：解析目标 userId → service →
// envelope。管理面语义与源一致（requirePerm 收口，威胁评审接受）。
func (h *AuthHandler) Kickout(c *gin.Context) {
	var req dto.KickoutReq
	if !bindJSON(c, &req) {
		return
	}
	if err := h.svc.Kickout(c.Request.Context(), int64(*req.UserID)); err != nil {
		resp.Fail(c, err)
		return
	}
	resp.OK(c, nil)
}
