package resp_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cool-wx/sys-iam-go/pkg/errcode"
	"github.com/cool-wx/sys-iam-go/pkg/resp"
)

func newTestCtx(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/users", nil)
	return c, w
}

func TestOKEnvelopeThreeKeys(t *testing.T) {
	c, w := newTestCtx(t)
	resp.OK(c, nil)
	require.Equal(t, http.StatusOK, w.Code)

	body := w.Body.String()
	assert.Contains(t, body, `"code":0`)
	assert.Contains(t, body, `"message":"ok"`)
	assert.Contains(t, body, `"data":null`, "data=null 时键必须保留（禁 omitempty）")
}

func TestOKWithData(t *testing.T) {
	c, w := newTestCtx(t)
	resp.OK(c, map[string]any{"id": "1"})
	assert.Contains(t, w.Body.String(), `"data":{"id":"1"}`)
}

func TestFailUnknownErrorMasksOriginal(t *testing.T) {
	// T-1-01 防泄漏：未知错误收敛 10001 固定文案，原文绝不出网
	c, w := newTestCtx(t)
	resp.Fail(c, errors.New("sql: boom"))
	require.Equal(t, http.StatusInternalServerError, w.Code)

	body := w.Body.String()
	assert.Contains(t, body, `"code":10001`)
	assert.Contains(t, body, `"data":null`)
	assert.NotContains(t, body, "boom", "原始错误原文不得出现在响应 body")
}

func TestFailErrcodePassthrough(t *testing.T) {
	c, w := newTestCtx(t)
	resp.Fail(c, errcode.ErrNotFound)
	require.Equal(t, http.StatusNotFound, w.Code)

	body := w.Body.String()
	assert.Contains(t, body, `"code":10300`)
	assert.Contains(t, body, `"message":"数据不存在"`)
}

func TestFailWrappedErrcode(t *testing.T) {
	// errors.As 链路：包装后的业务错误仍被收敛识别
	c, w := newTestCtx(t)
	wrapped := errcode.New(errcode.CodeInvalidParam, "用户名已存在")
	resp.Fail(c, wrapped)
	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), `"code":10400`)
}
