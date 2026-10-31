// Package errcode 定义全库统一的业务错误码：分段常量、错误类型与 HTTP 状态如实映射。
package errcode

// 错误码分段常量（锁定表，见 01-01-PLAN.md Task 1 第 4 步）。
const (
	CodeInternal        = 10001 // 内部错误
	CodeUnauthenticated = 10100 // 未登录
	CodeForbidden       = 10200 // 无权限
	CodeNotFound        = 10300 // 数据不存在
	CodeInvalidParam    = 10400 // 参数无效（含"记录已存在"）
	CodeValidation      = 10401 // 参数校验失败
	CodeBadCredentials  = 10501 // 用户名或密码错误
)

// 分段错误单例（消息文案为出网固定文案）。
var (
	ErrInternal        = New(CodeInternal, "服务器内部错误")
	ErrUnauthenticated = New(CodeUnauthenticated, "未登录")
	ErrForbidden       = New(CodeForbidden, "无权限")
	ErrNotFound        = New(CodeNotFound, "数据不存在")
	ErrInvalidParam    = New(CodeInvalidParam, "参数无效")
	ErrValidation      = New(CodeValidation, "参数校验失败")
	ErrBadCredentials  = New(CodeBadCredentials, "用户名或密码错误")
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

// HTTPStatus 错误码→HTTP 状态如实映射（未登录 401、无权限 403、参数 400、
// 不存在 404、内部 500；未知分段按内部错误兜底）。
func (e *Error) HTTPStatus() int {
	switch e.Code {
	case CodeInternal:
		return 500
	case CodeUnauthenticated, CodeBadCredentials:
		return 401
	case CodeForbidden:
		return 403
	case CodeNotFound:
		return 404
	case CodeInvalidParam, CodeValidation:
		return 400
	default:
		return 500
	}
}
