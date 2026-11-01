package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// MaxBodyMB 全局请求体上限（MB 数，MiB 语义）——单一事实源（IN-10）：字节上限
// maxBodyBytes 与 handler 超限文案均由它派生，调整上限只改这一处，用户可见文案
// 不会与真实上限漂移。按当前用户域 JSON 契约字段裕量设定；后续大 payload 域如
// Excel 导入应另设独立上限，不复用全局值。
const MaxBodyMB int64 = 1

// maxBodyBytes 请求体大小上限（字节）：由 MaxBodyMB 派生（编译期常量运算）。
const maxBodyBytes int64 = MaxBodyMB << 20

// BodyLimit 请求体大小上限中间件（WR-02/T-1-04 DoS 防护）：经 http.MaxBytesReader
// 包裹请求体，超限后读取返回 *http.MaxBytesError，由 handler.bindJSON 收敛为固定
// 文案 10401 envelope（本中间件不改写响应）。注意：MaxBytesReader 的
// requestTooLarge 连接标记需 ResponseWriter 实现对应接口，gin 的 responseWriter
// 未实现，该标记路径在 gin 下不生效（IN-09）——超限连接不做 413 强制关闭，复用
// 与否交由 net/http 对未排空 body 的内置策略（残量 >256KB 时响应写完即关连接）。
// 工控机内存有限，无上限 body 经 ShouldBindJSON 全量读入可致 OOM。
//
// multipart 跳过（IN-10 预留设计的落地形态，FILE-05/T-4-06）：全局 1MB 不动，
// Content-Type 前缀 multipart/ 的请求直接放行——大 payload 交由 upload/import
// 路由级独立上限（BodyLimitWithMB）管理。若不跳过，外层 1MB MaxBytesReader 会
// 先于路由级 20MB 截断上传（全局中间件先包一层 1MB，路由级再包 20MB 时内层
// 生效值恒为 1MB）。
func BodyLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.HasPrefix(c.ContentType(), "multipart/") {
			c.Next()
			return
		}
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBodyBytes)
		}
		c.Next()
	}
}

// BodyLimitWithMB 路由级独立请求体上限（MB 数，MiB 语义）：大 payload 域（文件
// 上传 20MB）在路由注册处叠加本中间件——全局 BodyLimit 对 multipart 已跳过，
// 此处上限是该类请求的唯一封顶（T-4-06 DoS 防护面）。
func BodyLimitWithMB(mb int64) gin.HandlerFunc {
	limit := mb << 20
	return func(c *gin.Context) {
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		}
		c.Next()
	}
}
