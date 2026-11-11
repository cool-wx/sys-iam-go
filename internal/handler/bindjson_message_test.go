package handler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/cool-wx/sys-iam-go/internal/middleware"
)

// jsonBodyReader 流式生成「{"a":"AAAA...」形态的超长 JSON body：头部分 literal
// + 余量以 'A' 填充（不在内存构造真实大字符串）。字符串值不闭合，json.Decoder
// 解析时持续读流直至 MaxBytesReader 超限——bindJSON 才能收到 *http.MaxBytesError
// （全零字节等非法 JSON 会在第一块就 SyntaxError，读不到超限点）。
type jsonBodyReader struct {
	head      string
	headPos   int
	remaining int
}

func (r *jsonBodyReader) Read(p []byte) (int, error) {
	if r.headPos < len(r.head) {
		n := copy(p, r.head[r.headPos:])
		r.headPos += n
		return n, nil
	}
	if r.remaining <= 0 {
		return 0, io.EOF
	}
	n := r.remaining
	if n > len(p) {
		n = len(p)
	}
	for i := 0; i < n; i++ {
		p[i] = 'A'
	}
	r.remaining -= n
	return n, nil
}

// serveBindJSON 构造挂全局 BodyLimit 的 POST 路由（与 router.go 装配形态一致），
// 下游 handler 直调 bindJSON，返回响应 envelope JSON 文本。
func serveBindJSON(t *testing.T, bodyBytes int, contentType string) string {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/v1/x", middleware.BodyLimit(), func(c *gin.Context) {
		var req struct {
			A string `json:"a"`
		}
		if !bindJSON(c, &req) {
			return
		}
		c.Status(http.StatusOK)
	})
	body := &jsonBodyReader{head: `{"a":"`, remaining: bodyBytes - len(`{"a":"`)}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/x", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec.Body.String()
}

// TestBindJSONMaxBytesMessageFromLimit 直证：超限文案由 MaxBytesError.Limit
// （实际触发的截断层上限）生成，不再按 middleware.MaxBodyMB 固定取值——
// multipart Content-Type 打 JSON 契约路由超全局 20MB 兜底时文案报
// 「20MB」，修复前固定报「1MB」与实际上限漂移（已知残留归零）。
// JSON 路由 1MB 既有文案不回归。
func TestBindJSONMaxBytesMessageFromLimit(t *testing.T) {
	// multipart 打 JSON 路由：全局 BodyLimit 对 multipart 套 MaxMultipartMB
	// 兜底封顶，20MB+1 的 body 读至超限点触发 Limit=20MB 的
	// MaxBytesError。
	body := serveBindJSON(t, int(middleware.MaxMultipartMB)<<20+1, "multipart/form-data; boundary=x")
	assert.Contains(t, body, "请求体超过 20MB 上限", "文案必须由实际触发的截断层 limit 生成")
	assert.NotContains(t, body, "1MB", "文案不得再按 MaxBodyMB 固定生成")

	// 既有 JSON 路由 1MB 文案不回归。
	body2 := serveBindJSON(t, int(middleware.MaxBodyMB)<<20+1, "application/json")
	assert.Contains(t, body2, "请求体超过 1MB 上限", "JSON 路由 1MB 文案不得回归")
}
