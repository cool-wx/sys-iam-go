// Package errcode 定义全库统一的业务错误码：分段常量、错误类型与 HTTP 状态如实映射。
package errcode

// 错误码分段常量（分段锁定表）。
const (
	CodeInternal        = 10001 // 内部错误
	CodeUnauthenticated = 10100 // 未登录
	CodeForbidden       = 10200 // 无权限
	CodeNotFound        = 10300 // 数据不存在
	CodeInvalidParam    = 10400 // 参数无效（含"记录已存在"）
	CodeValidation      = 10401 // 参数校验失败
	CodeBadCredentials  = 10501 // 用户名或密码错误
	// 认证增量：全部 HTTP 401（源消息可区分，独立错误码保持 errcode 语义清晰）。
	CodeUserDisabled       = 10502 // 用户已被禁用
	CodeApiKeyInvalid      = 10503 // API Key 无效或已过期
	CodeApiKeyNoUser       = 10504 // API Key 未关联用户
	CodeApiKeyUserMissing  = 10505 // API Key 关联用户不存在
	CodeApiKeyUserDisabled = 10506 // API Key 关联用户已被禁用
	// builtin 冲突段：HTTP 409（builtin 守卫统一语义）。
	CodeBuiltinUser = 10601 // 系统内置用户不可删除（User 域复用）
	CodeBuiltinRole = 10602 // 系统内置角色不可删除
	// builtin 分配守卫（提权收口）：HTTP 403——非 super_admin 操作者
	// 分配 builtin 角色属越权行为（非资源状态冲突），与 10601/10602 的 409 删除冲突
	// 语义区分。
	CodeBuiltinRoleAssign = 10603 // 系统内置角色仅 super_admin 可分配
	// builtin 停用守卫：HTTP 409——builtin 角色经 status 路径停用
	// 与「不可删除」同族资源状态冲突（builtin 守卫统一语义，对齐 10602），非
	// 参数错误；独立于 10602 因消息语义不同（删除 vs 停用）。
	CodeBuiltinRoleStatus = 10604 // 系统内置角色不可停用
)

// 分段错误单例（消息文案为出网固定文案）。
var (
	ErrInternal          = New(CodeInternal, "服务器内部错误")
	ErrUnauthenticated   = New(CodeUnauthenticated, "未登录")
	ErrForbidden         = New(CodeForbidden, "无权限")
	ErrNotFound          = New(CodeNotFound, "数据不存在")
	ErrInvalidParam      = New(CodeInvalidParam, "参数无效")
	ErrValidation        = New(CodeValidation, "参数校验失败")
	ErrBadCredentials    = New(CodeBadCredentials, "用户名或密码错误")
	ErrUserDisabled      = New(CodeUserDisabled, "用户已被禁用")
	ErrApiKeyInvalid     = New(CodeApiKeyInvalid, "API Key 无效或已过期")
	ErrBuiltinUser       = New(CodeBuiltinUser, "系统内置用户不可删除")
	ErrBuiltinRole       = New(CodeBuiltinRole, "系统内置角色不可删除")
	ErrBuiltinRoleAssign = New(CodeBuiltinRoleAssign, "系统内置角色仅超级管理员可分配")
	ErrBuiltinRoleStatus = New(CodeBuiltinRoleStatus, "系统内置角色不可停用")
)

// Error 业务错误类型：经调用链传播，由 pkg/resp 统一出口写出。
type Error struct {
	Code int
	Msg  string
}

// New 构造业务错误。
func New(code int, msg string) *Error {
	return &Error{Code: code, Msg: msg}
}

// Error 实现 error 接口。
func (e *Error) Error() string {
	return e.Msg
}

// HTTPStatus 错误码→HTTP 状态如实映射（未登录/凭据类 401、无权限 403、参数 400、
// 不存在 404、内部 500；未知分段按内部错误兜底）。
func (e *Error) HTTPStatus() int {
	switch e.Code {
	case CodeInternal:
		return 500
	case CodeUnauthenticated, CodeBadCredentials,
		CodeUserDisabled, CodeApiKeyInvalid, CodeApiKeyNoUser, CodeApiKeyUserMissing, CodeApiKeyUserDisabled:
		return 401
	case CodeForbidden, CodeBuiltinRoleAssign:
		return 403
	case CodeBuiltinUser, CodeBuiltinRole, CodeBuiltinRoleStatus:
		return 409
	case CodeNotFound:
		return 404
	case CodeInvalidParam, CodeValidation:
		return 400
	default:
		return 500
	}
}
