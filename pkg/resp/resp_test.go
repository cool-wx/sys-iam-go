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
	// 防泄漏：未知错误收敛 10001 固定文案，原文绝不出网
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

// TestFailExposesErrorToGinErrors Fail 经 c.Error 把原错误对象透出到 gin 私有
// errors（error_msg 消费链路）：len(c.Errors)==1、类型私有（不出响应体）、
// 消息为原 err 文案；响应体与透出前逐字节一致（既有 envelope 断言不回归）。
func TestFailExposesErrorToGinErrors(t *testing.T) {
	c, w := newTestCtx(t)
	resp.Fail(c, errcode.New(errcode.CodeInvalidParam, "参数错误"))

	require.Len(t, c.Errors, 1, "Fail 必须经 c.Error 透出原错误对象")
	assert.Equal(t, gin.ErrorTypePrivate, c.Errors[0].Type, "透出错误必须私有（不出响应体）")
	require.NotNil(t, c.Errors[0].Err)
	assert.Equal(t, "参数错误", c.Errors[0].Err.Error(), "透出消息必须为原 err 文案")
	assert.Contains(t, w.Body.String(), `"message":"参数错误"`)

	// 未知错误收敛路径同样透出原对象（Oplog 取 slog 前的原始 err 用于审计）
	c2, _ := newTestCtx(t)
	resp.Fail(c2, errors.New("sql: boom"))
	require.Len(t, c2.Errors, 1)
	assert.Equal(t, "sql: boom", c2.Errors[0].Err.Error())

	// nil err 防御：不产生 gin error
	c3, _ := newTestCtx(t)
	resp.Fail(c3, nil)
	assert.Len(t, c3.Errors, 0)
}
