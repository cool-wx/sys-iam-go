// Package resp 定义全库统一响应 envelope 出口：{code, message, data} 三键恒在
// （无 omitempty，data 键在成功与失败响应中均存在）。
package resp

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/cool-wx/sys-iam-go/pkg/errcode"
)

// Envelope 统一响应包装。三键均无 omitempty——失败时 data 以 null 键输出。
type Envelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

// OK 写出成功 envelope（code=0，message="ok"，HTTP 200）。
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Envelope{Code: 0, Message: "ok", Data: data})
}

// Fail 写出失败 envelope：errcode.Error 按 Code/HTTPStatus 写出；
// 非业务错误收敛为 10001 固定文案，原始错误仅 slog Error 记录（防泄漏，T-1-01）。
func Fail(c *gin.Context, err error) {
	var ec *errcode.Error
	if !errors.As(err, &ec) {
		slog.Error("unhandled error", "error", err.Error())
		ec = errcode.ErrInternal
	}
	c.JSON(ec.HTTPStatus(), Envelope{Code: ec.Code, Message: ec.Msg, Data: nil})
}
