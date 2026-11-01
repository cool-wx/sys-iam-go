// RequirePerm 中间件单元测试（fake Authorizer + httptest，无 build tag 免容器）。
// 表格驱动四分支：identity 缺失 401、未授权 403/10200「无权限」、enforcer 故障
// 500 fail-closed、命中放行 200（03-PATTERNS 第 10 节，照 auth_test.go 风格）。
package middleware

import (
	"encoding/json"
	"errors"
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
