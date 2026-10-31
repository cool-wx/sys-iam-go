package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// maxBodyBytes 请求体大小上限：1MB（按当前用户域 JSON 契约字段裕量设定；后续
// 大 payload 域如 Excel 导入应另设独立上限，不复用全局值）。
const maxBodyBytes int64 = 1 << 20

// BodyLimit 请求体大小上限中间件（WR-02/T-1-04 DoS 防护）：经 http.MaxBytesReader
// 包裹请求体，超限后读取返回 *http.MaxBytesError，由 handler.bindJSON 收敛为固定
// 文案 10401 envelope（本中间件不改写响应）。注意：MaxBytesReader 的
// requestTooLarge 连接标记需 ResponseWriter 实现对应接口，gin 的 responseWriter
// 未实现，该标记路径在 gin 下不生效（IN-09）——超限连接不做 413 强制关闭，复用
// 与否交由 net/http 对未排空 body 的内置策略（残量 >256KB 时响应写完即关连接）。
// 工控机内存有限，无上限 body 经 ShouldBindJSON 全量读入可致 OOM。
func BodyLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBodyBytes)
		}
		c.Next()
	}
}
