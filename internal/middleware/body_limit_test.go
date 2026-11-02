package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sizedReader 定长假请求体：按需生成 N 字节流，不在内存构造真实大字符串
// （测试需 20MB+ 量级，真实字符串会白白占用堆）。
type sizedReader struct{ remaining int }

func (r *sizedReader) Read(p []byte) (int, error) {
	if r.remaining <= 0 {
		return 0, io.EOF
	}
	n := r.remaining
	if n > len(p) {
		n = len(p)
	}
	r.remaining -= n
	return n, nil
}

// serveWithBody 构造挂给定中间件链的路由并发出 bodyBytes 大小的请求，下游 handler
// 全量读 body 并经闭包捕获「读到的字节数 + 错误」返回（MaxBytesError 直证用）。
func serveWithBody(t *testing.T, bodyBytes int, contentType string, handlers ...gin.HandlerFunc) (int, error) {
	t.Helper()
	var (
		gotN   int
		gotErr error
	)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/v1/x", append(handlers, func(c *gin.Context) {
		n, err := io.Copy(io.Discard, c.Request.Body)
		gotN, gotErr = int(n), err
		c.Status(http.StatusOK)
	})...)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/x", &sizedReader{remaining: bodyBytes})
	req.Header.Set("Content-Type", contentType)
	router.ServeHTTP(httptest.NewRecorder(), req)
	return gotN, gotErr
}

// TestBodyLimitMultipartBoundedOnNonUploadRoutes multipart 无界读入修复直证：
// Content-Type 打无路由级上限的普通路由（免认证 login 形态），全局 BodyLimit
// 必须以 MaxMultipartMB 兜底封顶——下游读到 MaxBytesError，body 不被无界读入
// （修复前：multipart 分支直接 c.Next() 放行，任意大 body 被 ShouldBindJSON
// 全量读入内存）。
func TestBodyLimitMultipartBoundedOnNonUploadRoutes(t *testing.T) {
	_, err := serveWithBody(t, 21<<20+1, "multipart/form-data; boundary=x", BodyLimit())
	require.Error(t, err, "21MB multipart body 不得被完整读入")
	var mbe *http.MaxBytesError
	assert.ErrorAs(t, err, &mbe, "超限必须是 MaxBytesError（bindJSON 10401 文案路径）")
}

// TestBodyLimitMultipartUploadRouteDoubleCap 双层封顶叠加直证（等价 upload/import
// 路由形态 BodyLimit + BodyLimitWithMB(MaxMultipartMB)）：内层生效值不变——
// 20MB 内完整可读（合法大上传不回归），超 20MB 仍 MaxBytesError。
func TestBodyLimitMultipartUploadRouteDoubleCap(t *testing.T) {
	// 15MB < 20MB：完整可读
	n, err := serveWithBody(t, 15<<20, "multipart/form-data; boundary=x",
		BodyLimit(), BodyLimitWithMB(MaxMultipartMB))
	require.NoError(t, err, "15MB 合法 multipart 上传不得被截断")
	assert.Equal(t, 15<<20, n, "15MB body 必须完整到达 handler")

	// 25MB > 20MB：封顶生效
	_, err = serveWithBody(t, 25<<20, "multipart/form-data; boundary=x",
		BodyLimit(), BodyLimitWithMB(MaxMultipartMB))
	require.Error(t, err, "25MB multipart 必须被 20MB 封顶")
	var mbe *http.MaxBytesError
	assert.ErrorAs(t, err, &mbe)
}

// TestBodyLimitJSONRoutesUnchanged 非 multipart 路由 1MB 上限不回归（修复
// 不得改动既有 JSON 防护面）：恰好 1MB 完整可读，1MB+1 触发 MaxBytesError。
func TestBodyLimitJSONRoutesUnchanged(t *testing.T) {
	n, err := serveWithBody(t, int(maxBodyBytes), "application/json", BodyLimit())
	require.NoError(t, err, "恰好 1MB 的 JSON body 必须完整可读")
	assert.Equal(t, int(maxBodyBytes), n)

	_, err = serveWithBody(t, int(maxBodyBytes)+1, "application/json", BodyLimit())
	require.Error(t, err, "超 1MB JSON body 必须被 MaxBytesReader 截断")
	var mbe *http.MaxBytesError
	assert.ErrorAs(t, err, &mbe)
}
