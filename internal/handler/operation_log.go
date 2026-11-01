// Package handler 操作日志查询域 HTTP handler（薄层三段式；分页入口是全库唯一
// ParseWithDefaults 特例调用点）。
package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/cool-wx/sys-iam-go/internal/service"
	"github.com/cool-wx/sys-iam-go/pkg/paging"
	"github.com/cool-wx/sys-iam-go/pkg/resp"
)

// OlogHandler 操作日志查询域 HTTP handler。
type OlogHandler struct {
	svc *service.OperationLogService
}

// NewOlogHandler 构造。
func NewOlogHandler(svc *service.OperationLogService) *OlogHandler {
	return &OlogHandler{svc: svc}
}

// List 处理 GET /api/v1/logs：page/size 经 ParseWithDefaults 特例钳位（缺省 1/10
// ——源 LogQuery DTO 字段默认值，D-09 拍板；OLOG-03 验收明文「特例，非全局 1/15」，
// 畸形输入统一归缺省 10）；module/operation/real_name 三 like 模糊（空串=不过滤）
// → service → envelope（items 按 create_time 倒序，空结果 [] 非 null）。
func (h *OlogHandler) List(c *gin.Context) {
	page, size := paging.ParseWithDefaults(c.Query("page"), c.Query("size"), 1, 10)
	data, err := h.svc.List(c.Request.Context(), page, size,
		c.Query("module"), c.Query("operation"), c.Query("real_name"))
	if err != nil {
		resp.Fail(c, err)
		return
	}
	resp.OK(c, data)
}
