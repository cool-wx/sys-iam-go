package errcode_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cool-wx/sys-iam-go/pkg/errcode"
)

func TestHTTPStatusMapping(t *testing.T) {
	// 七分段锁定表（Task 1 第 4 步定案）
	cases := []struct {
		code int
		want int
	}{
		{errcode.CodeInternal, 500},
		{errcode.CodeUnauthenticated, 401},
		{errcode.CodeForbidden, 403},
		{errcode.CodeNotFound, 404},
		{errcode.CodeInvalidParam, 400},
		{errcode.CodeValidation, 400},
		{errcode.CodeBadCredentials, 401},
		// Phase 2 builtin 冲突段（D-14）：409
		{errcode.CodeBuiltinUser, 409},
		{errcode.CodeBuiltinRole, 409},
		// Phase 3 builtin 分配守卫（CR-03）：越权行为译 403，非删除冲突 409
		{errcode.CodeBuiltinRoleAssign, 403},
		// Phase 3 builtin 停用守卫（WR-06）：与删除守卫同族资源状态冲突，409
		{errcode.CodeBuiltinRoleStatus, 409},
	}
	for _, tc := range cases {
		e := errcode.New(tc.code, "任意文案")
		assert.Equal(t, tc.want, e.HTTPStatus(), "code %d → HTTP %d", tc.code, tc.want)
	}
}

func TestHTTPStatusUnknownCodeDefaultsInternal(t *testing.T) {
	e := errcode.New(19999, "未定义分段")
	assert.Equal(t, 500, e.HTTPStatus(), "未知分段按内部错误兜底")
}

func TestErrorMethodReturnsMsg(t *testing.T) {
	assert.Equal(t, "数据不存在", errcode.ErrNotFound.Error())
	assert.Equal(t, "服务器内部错误", errcode.ErrInternal.Error())
}

func TestNewConstructsFields(t *testing.T) {
	e := errcode.New(errcode.CodeInvalidParam, "用户名已存在")
	require.NotNil(t, e)
	assert.Equal(t, errcode.CodeInvalidParam, e.Code)
	assert.Equal(t, "用户名已存在", e.Msg)
}
