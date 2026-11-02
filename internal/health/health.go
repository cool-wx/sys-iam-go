// Package health 提供 liveness/readiness 端点与关停摘流开关。
// body 恒为 {"status":"ok"|"fail"}——不含 DSN/主机/版本等内部细节。
package health

import (
	"context"
	"database/sql"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

// State 健康状态：持有 MySQL 连接与关停摘流标志。
type State struct {
	sqlDB    *sql.DB
	notReady atomic.Bool
}

// New 构造健康状态（sqlDB 供 readiness ping）。
func New(sqlDB *sql.DB) *State {
	return &State{sqlDB: sqlDB}
}

// SetNotReady 立即摘流（优雅关停第一步：readyz 转 503）。
func (s *State) SetNotReady() {
	s.notReady.Store(true)
}

// Liveness 处理 GET /healthz：进程在即 200（恒 true）。
func (s *State) Liveness(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// Readiness 处理 GET /readyz：摘流后或 MySQL ping 失败（1s 超时）→ 503。
func (s *State) Readiness(c *gin.Context) {
	if s.notReady.Load() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "fail"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), time.Second)
	defer cancel()
	if err := s.sqlDB.PingContext(ctx); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "fail"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
