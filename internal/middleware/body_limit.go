package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// MaxBodyMB 全局请求体上限（MB 数，MiB 语义）——唯一常量事实依据：字节上限
// maxBodyBytes 与 handler 超限文案均由它派生，调整上限只改这一处，用户可见文案
// 不会与真实上限漂移。按当前用户域 JSON 契约字段裕量设定；后续大 payload 域如
// Excel 导入应另设独立上限，不复用全局值。
const MaxBodyMB int64 = 1

// maxBodyBytes 请求体大小上限（字节）：由 MaxBodyMB 派生（编译期常量运算）。
const maxBodyBytes int64 = MaxBodyMB << 20

// MaxMultipartMB multipart 请求体上限（MB 数，MiB 语义）——唯一常量事实依据：
// 全局 BodyLimit 对 multipart 的兜底封顶与 upload/import 路由级 BodyLimitWithMB
// 独立上限共用本常量，调整上限只改这一处，两层永不漂移。
const MaxMultipartMB int64 = 20

// maxMultipartBytes multipart 请求体大小上限（字节）：由 MaxMultipartMB 派生。
const maxMultipartBytes int64 = MaxMultipartMB << 20

// BodyLimit 请求体大小上限中间件（DoS 防护）：经 http.MaxBytesReader
// 包裹请求体，超限后读取返回 *http.MaxBytesError，由 handler.bindJSON 收敛为固定
// 文案 10401 envelope（本中间件不改写响应）。注意：MaxBytesReader 的
// requestTooLarge 连接标记需 ResponseWriter 实现对应接口，gin 的 responseWriter
// 未实现，该标记路径在 gin 下不生效——超限连接不做 413 强制关闭，复用
// 与否交由 net/http 对未排空 body 的内置策略（残量 >256KB 时响应写完即关连接）。
// 工控机内存有限，无上限 body 经 ShouldBindJSON 全量读入可致 OOM。
//
// multipart 分支（预留设计的落地形态；收紧）：全局
// 1MB 不套（否则外层 1MB MaxBytesReader 会先于路由级 20MB 截断上传——全局中间件
// 先包一层 1MB，路由级再包 20MB 时内层生效值恒为 1MB），改套同值 20MB 兜底上限。
// 豁免必须是「上限豁免」而非「防护豁免」：跳过判定按 Content-Type 前缀而非按
// 路由，若 multipart 直接放行，声明 multipart Content-Type 的请求可打任意无路由
// 级上限的 JSON 路由（如免认证 POST /api/v1/auth/login）无界读入内存致 OOM。
// 与 upload/import 路由级 BodyLimitWithMB(MaxMultipartMB) 同值叠加时
// 内层生效值不变（均为 20MB）。
func BodyLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body != nil {
			limit := maxBodyBytes
			if strings.HasPrefix(c.ContentType(), "multipart/") {
				limit = maxMultipartBytes
			}
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		}
		c.Next()
	}
}

// BodyLimitWithMB 路由级独立请求体上限（MB 数，MiB 语义）：大 payload 域（文件
// 上传/Excel 导入 20MB）在路由注册处叠加本中间件。全局 BodyLimit 对 multipart
// 不套 1MB 而套同值 20MB 兜底——本中间件与该兜底同值叠加，内层生效值
// 不变，仍为大 payload 请求的实际封顶（DoS 防护面）。
func BodyLimitWithMB(mb int64) gin.HandlerFunc {
	limit := mb << 20
	return func(c *gin.Context) {
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		}
		c.Next()
	}
}
