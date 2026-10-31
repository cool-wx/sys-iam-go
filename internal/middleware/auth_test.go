// 认证中间件单元测试（miniredis + httptest，无 build tag）。
// 三分支 401 Abort + 有效 token 注入 Identity + Redis 故障 fail-closed 500 直证。
package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cool-wx/sys-iam-go/internal/auth"
)

// newAuthTestEnv 起 miniredis + SessionManager + 挂 Auth 中间件的 gin engine。
// 探针 handler 回显 auth.FromContext 的 identity（注入正确性的直证载体）。
func newAuthTestEnv(t *testing.T) (*gin.Engine, *auth.SessionManager, *miniredis.Miniredis) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	sm := auth.NewSessionManager(rdb, 30*time.Minute)

	engine := gin.New()
	engine.POST("/probe", Auth(sm), func(c *gin.Context) {
		id, ok := auth.FromContext(c.Request.Context())
		if !ok {
			c.JSON(http.StatusOK, gin.H{"identity": false})
			return
		}
		c.JSON(http.StatusOK, gin.H{"identity": true, "userId": id.UserID, "name": id.Name})
	})
	return engine, sm, mr
}

// doProbe 发 POST /probe，可带 Authorization 头；返回 status 与 body。
func doProbe(t *testing.T, engine *gin.Engine, authHeader string) (int, []byte) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/probe", nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w.Code, w.Body.Bytes()
}

func TestAuthMiddlewareRejects(t *testing.T) {
	engine, _, _ := newAuthTestEnv(t)

	cases := []struct {
		name string
		hdr  string
	}{
		{"无 Authorization 头", ""},
		{"非 Bearer 前缀", "Token abc123"},
		{"Bearer 空 token", "Bearer "},
		{"Bearer 纯空白", "Bearer    "},
		{"无效 token（会话不存在）", "Bearer deadbeef"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, body := doProbe(t, engine, tc.hdr)
			require.Equal(t, http.StatusUnauthorized, status, "body=%s", body)
			var envelope struct {
				Code int    `json:"code"`
				Data any    `json:"data"`
				Msg  string `json:"message"`
			}
			require.NoError(t, json.Unmarshal(body, &envelope))
			assert.Equal(t, 10100, envelope.Code, "未登录必须落 10100: body=%s", body)
			assert.Nil(t, envelope.Data)
			assert.NotEmpty(t, envelope.Msg)
		})
	}
}

// TestAuthMiddlewarePassesIdentity 有效 token：200 且探针 handler 经 auth.FromContext
// 拿到正确 Identity（realName 优先语义）。
func TestAuthMiddlewarePassesIdentity(t *testing.T) {
	engine, sm, _ := newAuthTestEnv(t)
	token, err := sm.Login(t.Context(), auth.SessionPayload{
		UserID: 66, UserName: "mw-user", RealName: "中间件用户", Roles: []string{"ops"},
	})
	require.NoError(t, err)

	status, body := doProbe(t, engine, "Bearer "+token)
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	var got struct {
		Identity bool   `json:"identity"`
		UserID   int64  `json:"userId"`
		Name     string `json:"name"`
	}
	require.NoError(t, json.Unmarshal(body, &got))
	assert.True(t, got.Identity, "identity 必须经 WithIdentity 注入 ctx")
	assert.Equal(t, int64(66), got.UserID)
	assert.Equal(t, "中间件用户", got.Name)
}

// TestAuthMiddlewareFailsClosedOnRedisError miniredis Close 后请求得 500
// （T-2-04 fail-closed 直证：Redis 故障绝不静默放行）。
func TestAuthMiddlewareFailsClosedOnRedisError(t *testing.T) {
	engine, sm, mr := newAuthTestEnv(t)
	token, err := sm.Login(t.Context(), auth.SessionPayload{UserID: 1, UserName: "fc-user"})
	require.NoError(t, err)

	mr.Close() // 模拟 Redis 故障：会话不可查

	status, body := doProbe(t, engine, "Bearer "+token)
	require.Equal(t, http.StatusInternalServerError, status, "Redis 故障必须 fail-closed 500: body=%s", body)
	var envelope struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(body, &envelope))
	assert.Equal(t, 10001, envelope.Code, "body=%s", body)
}

// TestBearerToken 提取函数边界：合法 Bearer、大小写敏感（非 Bearer 前缀拒绝）、
// 空白 token 拒绝。
func TestBearerToken(t *testing.T) {
	tok, ok := BearerToken("Bearer abc")
	assert.True(t, ok)
	assert.Equal(t, "abc", tok)

	_, ok = BearerToken("bearer abc") // 前缀大小写敏感（RFC 7235 scheme 大小写不敏感的宽容实现不引入——源/前端均用标准 Bearer）
	assert.False(t, ok)
	_, ok = BearerToken("")
	assert.False(t, ok)
	tok, ok = BearerToken("Bearer   padded   ")
	assert.True(t, ok)
	assert.Equal(t, "padded", tok, "token 两端空白应被修剪")
}
