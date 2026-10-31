// Package middleware 提供 Gin 中间件链：RequestID → AccessLog → Recovery（顺序固定）。
package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
)

// requestIDCtxKey 私有 ctx key。
type requestIDCtxKey struct{}

// RequestID 请求 ID 中间件：入站 X-Request-ID 有则复用，无则 crypto/rand 16 字节 hex；
// 写入响应 header 与请求 ctx。
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := c.GetHeader("X-Request-ID")
		if rid == "" {
			b := make([]byte, 16)
			if _, err := rand.Read(b); err != nil {
				// crypto/rand 失败极罕见；时间戳兜底保请求链可追踪
				rid = fmt.Sprintf("%x", time.Now().UnixNano())
			} else {
				rid = hex.EncodeToString(b)
			}
		}
		c.Set("request_id", rid)
		c.Header("X-Request-ID", rid)
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), requestIDCtxKey{}, rid))
		c.Next()
	}
}

// RequestIDFromCtx 从标准 ctx 取请求 ID。
func RequestIDFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDCtxKey{}).(string); ok {
		return v
	}
	return ""
}
