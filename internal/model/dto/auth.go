package dto

import "github.com/cool-wx/sys-iam-go/pkg/ids"

// LoginReq 用户名密码登录请求（D-01：POST /api/v1/auth/login {username,password}）。
type LoginReq struct {
	Username *string `json:"username" binding:"required"`
	Password *string `json:"password" binding:"required"`
}

// LoginResp 登录响应（D-01：{token, user}）。User 复用 UserResp 契约——
// password/extToken 永不出网（T-2-05）。
type LoginResp struct {
	Token string    `json:"token"`
	User  *UserResp `json:"user"`
}

// ApiKeyLoginReq API Key 登录请求（D-03：POST /api/v1/auth/apikey-login {apiKey}）。
type ApiKeyLoginReq struct {
	ApiKey *string `json:"apiKey" binding:"required"`
}

// KickoutReq 踢下线请求（AUTH-04：POST /api/v1/auth/kickout {userId}）。
type KickoutReq struct {
	UserID *ids.ID `json:"userId" binding:"required"`
}
