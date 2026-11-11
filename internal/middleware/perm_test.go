// RequirePerm 中间件单元测试（fake Authorizer + httptest，无 build tag 免容器）。
// 表格驱动四分支：identity 缺失 401、未授权 403/10200「无权限」、enforcer 故障
// 500 fail-closed、命中放行 200（与 auth_test.go 同风格）。
package middleware

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cool-wx/sys-iam-go/internal/auth"
	"github.com/cool-wx/sys-iam-go/internal/authz"
)

// fakeAuthorizer 可编程 Authorizer：allowed/err 决定 Enforce 行为，calls 记录
// 调用参数（sub 为 userID 十进制串的直证载体）。
type fakeAuthorizer struct {
	allowed bool
	err     error
	calls   int
	lastSub string
}

func (f *fakeAuthorizer) Enforce(sub string, perm string) (bool, error) {
	f.calls++
	f.lastSub = sub
	return f.allowed, f.err
}

// newPermTestEnv 挂 Auth+RequirePerm 的 gin engine：enforcer 为 fake，
// identity 由 WithIdentity 直注（Auth 中间件不在被测面——中间件形态单测）。
func newPermTestEnv(t *testing.T, az authz.Authorizer, perm string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/guarded", func(c *gin.Context) {
		if c.GetHeader("Authorization") != "" {
			c.Request = c.Request.WithContext(auth.WithIdentity(c.Request.Context(),
				auth.Identity{UserID: 42, Name: "mw-user"}))
		}
		c.Next()
	}, RequirePerm(az, perm), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	return engine
}

func doPermProbe(t *testing.T, engine *gin.Engine, authHeader string) (int, []byte) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/guarded", nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w.Code, w.Body.Bytes()
}

func TestRequirePermMiddleware(t *testing.T) {
	cases := []struct {
		name       string
		authHeader string
		az         *fakeAuthorizer
		wantStatus int
		wantCode   int
		wantMsg    string
	}{
		{
			name:       "identity 缺失 401 防御分支",
			authHeader: "",
			az:         &fakeAuthorizer{allowed: true},
			wantStatus: http.StatusUnauthorized,
			wantCode:   10100,
			wantMsg:    "未登录",
		},
		{
			name:       "Enforce false 403 无权限",
			authHeader: "Bearer tok",
			az:         &fakeAuthorizer{allowed: false},
			wantStatus: http.StatusForbidden,
			wantCode:   10200,
			wantMsg:    "无权限",
		},
		{
			name:       "Enforce error 500 fail-closed",
			authHeader: "Bearer tok",
			az:         &fakeAuthorizer{err: errors.New("enforcer down")},
			wantStatus: http.StatusInternalServerError,
			wantCode:   10001,
			wantMsg:    "服务器内部错误",
		},
		{
			name:       "Enforce true 放行 200",
			authHeader: "Bearer tok",
			az:         &fakeAuthorizer{allowed: true},
			wantStatus: http.StatusOK,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine := newPermTestEnv(t, tc.az, "system:user:list")
			status, body := doPermProbe(t, engine, tc.authHeader)
			require.Equal(t, tc.wantStatus, status, "body=%s", body)
			var env struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			}
			require.NoError(t, json.Unmarshal(body, &env), "body=%s", body)
			if tc.wantCode != 0 {
				assert.Equal(t, tc.wantCode, env.Code, "body=%s", body)
				assert.Equal(t, tc.wantMsg, env.Message, "body=%s", body)
			}

			// 放行/拒绝路径必须真正调用了 enforcer（identity 缺失分支不触碰）
			if tc.authHeader != "" {
				require.Equal(t, 1, tc.az.calls, "带 identity 的请求必须经 enforcer 判定")
				assert.Equal(t, "42", tc.az.lastSub, "sub 必须为 userID 十进制串")
			}
		})
	}
}

// TestRequirePermEnforcerErrorLogs enforcer 故障分支必须 slog.Error 留痕（回归）：
// 授权链故障是 IAM 的最高严重级事件，全站无差别 500 时日志必须含根因可归因三键——
// 消息、sub（userID 十进制串）、perm 串、error 原文；不得含敏感值（token/请求体）。
func TestRequirePermEnforcerErrorLogs(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	rootCause := errors.New("gorm-adapter connection refused")
	engine := newPermTestEnv(t, &fakeAuthorizer{err: rootCause}, "system:user:list")
	status, body := doPermProbe(t, engine, "Bearer tok")

	require.Equal(t, http.StatusInternalServerError, status, "fail-closed 500 行为不变: body=%s", body)
	logs := buf.String()
	require.Contains(t, logs, "enforcer enforce failed", "日志必须含可检索消息: %s", logs)
	require.Contains(t, logs, "sub=42", "日志必须含 sub 归因键: %s", logs)
	require.Contains(t, logs, "perm=system:user:list", "日志必须含 perm 串: %s", logs)
	require.Contains(t, logs, rootCause.Error(), "日志必须含 error 根因原文: %s", logs)
	assert.NotContains(t, logs, "Bearer", "日志不得含 token 等敏感值")
}
