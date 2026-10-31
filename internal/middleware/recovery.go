// Package middleware panic 兜底中间件：统一 envelope 500 固定文案。
// panic 详情（含堆栈）只进 slog，不出网（防泄漏，等价源 GlobalExceptionHandler 兜底语义）。
package middleware

import (
	"fmt"
	"log/slog"
	"runtime/debug"

	"github.com/gin-gonic/gin"

	"github.com/cool-wx/sys-iam-go/pkg/errcode"
	"github.com/cool-wx/sys-iam-go/pkg/resp"
)

// Recovery panic → slog Error（含堆栈）→ envelope 10001。
func Recovery(logger *slog.Logger) gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered any) {
		logger.Error("panic recovered",
			"panic", fmt.Sprintf("%v", recovered),
			"stack", string(debug.Stack()),
			"request_id", c.GetString("request_id"),
		)
		resp.Fail(c, errcode.ErrInternal)
		c.Abort()
	})
}
