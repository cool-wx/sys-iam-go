// Package middleware 访问日志中间件：slog JSON 结构化输出。
// 字段白名单 method/path/status/duration_ms/request_id——绝不记录请求体与查询串。
package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

// AccessLog 请求后置日志：c.Next() 完成后按白名单五字段输出 Info。
func AccessLog(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		logger.Info("http",
			"method", c.Request.Method,
			"path", c.Request.URL.Path, // 仅 path，不含查询串
			"status", c.Writer.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
			"request_id", c.GetString("request_id"),
		)
	}
}
