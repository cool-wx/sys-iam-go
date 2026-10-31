package dto

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
