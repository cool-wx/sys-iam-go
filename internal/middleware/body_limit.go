package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// maxBodyBytes 请求体大小上限：1MB（按当前用户域 JSON 契约字段裕量设定；后续
// 大 payload 域如 Excel 导入应另设独立上限，不复用全局值）。
const maxBodyBytes int64 = 1 << 20

// BodyLimit 请求体大小上限中间件（WR-02/T-1-04 DoS 防护）：经 http.MaxBytesReader
// 包裹请求体，超限后读取返回 *http.MaxBytesError，且连接被标记 requestTooLarge
// （响应写完即关闭连接，413 传输语义）。工控机内存有限，无上限 body 经
// ShouldBindJSON 全量读入可致 OOM。超限读取错误在 handler 绑定路径收敛为固定
// 文案 envelope（见 handler.bindJSON），本中间件不改写响应。
func BodyLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBodyBytes)
		}
		c.Next()
	}
}
