// Package handler 故障域 HTTP handler（薄层三段式；List 走 paging.Parse 1/15
// 全局默认——源 fault list 走 PageHelper 同 1/15；Import 为 multipart 上传
// FAUL-02）。
package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/cool-wx/sys-iam-go/internal/model/dto"
	"github.com/cool-wx/sys-iam-go/internal/service"
	"github.com/cool-wx/sys-iam-go/pkg/errcode"
	"github.com/cool-wx/sys-iam-go/pkg/paging"
	"github.com/cool-wx/sys-iam-go/pkg/resp"
)

// FaultHandler 故障域 HTTP handler。
type FaultHandler struct {
	svc *service.FaultService
}

// NewFaultHandler 构造。
func NewFaultHandler(svc *service.FaultService) *FaultHandler {
	return &FaultHandler{svc: svc}
}

// Create 处理 POST /api/v1/faults：绑定校验 → service → envelope 出口。
func (h *FaultHandler) Create(c *gin.Context) {
	var req dto.FaultCreateReq
	if !bindJSON(c, &req) {
		return
	}
	data, err := h.svc.Create(c.Request.Context(), &req)
	if err != nil {
		resp.Fail(c, err)
		return
	}
	resp.OK(c, data)
}

// Update 处理 PUT /api/v1/faults/:id：解析 :id → 绑定指针 DTO → service → envelope。
func (h *FaultHandler) Update(c *gin.Context) {
	id, ok := parseID(c, "无效的故障 ID")
	if !ok {
		return
	}
	var req dto.FaultUpdateReq
	if !bindJSON(c, &req) {
		return
	}
	data, err := h.svc.Update(c.Request.Context(), id, &req)
	if err != nil {
		resp.Fail(c, err)
		return
	}
	resp.OK(c, data)
}

// Get 处理 GET /api/v1/faults/:id：解析 :id → service → envelope。
func (h *FaultHandler) Get(c *gin.Context) {
	id, ok := parseID(c, "无效的故障 ID")
	if !ok {
		return
	}
	data, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		resp.Fail(c, err)
		return
	}
	resp.OK(c, data)
}

// List 处理 GET /api/v1/faults：paging.Parse 1/15 全局默认（源 PageHelper 同
// 1/15）；code 可选 eq 直传过滤（idx_code 索引列）。
func (h *FaultHandler) List(c *gin.Context) {
	page, size := paging.Parse(c.Query("page"), c.Query("size"))
	data, err := h.svc.List(c.Request.Context(), page, size, c.Query("code"))
	if err != nil {
		resp.Fail(c, err)
		return
	}
	resp.OK(c, data)
}

// Delete 处理 DELETE /api/v1/faults/:id：解析 :id → service（物理删）→ envelope。
func (h *FaultHandler) Delete(c *gin.Context) {
	id, ok := parseID(c, "无效的故障 ID")
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), id); err != nil {
		resp.Fail(c, err)
		return
	}
	resp.OK(c, nil)
}

// Import 处理 POST /api/v1/faults/import（FAUL-02 multipart；路由层
// BodyLimitWithMB(20) 独立上限 + RequirePerm(PermFaultImport) + Oplog 三中间件
// 齐挂）：FormFile("file") 取上传流 → service 导入链（8 列契约 + 行数/表头双
// 前置防线 + 三键 upsert 整批事务）→ ImportResult envelope。
func (h *FaultHandler) Import(c *gin.Context) {
	fh, err := c.FormFile("file")
	if err != nil {
		resp.Fail(c, errcode.New(errcode.CodeInvalidParam, "缺少上传文件字段 file"))
		return
	}
	f, err := fh.Open()
	if err != nil {
		resp.Fail(c, errcode.New(errcode.CodeInvalidParam, "打开上传文件失败"))
		return
	}
	defer f.Close()
	result, err := h.svc.ImportExcel(c.Request.Context(), f)
	if err != nil {
		resp.Fail(c, err)
		return
	}
	resp.OK(c, result)
}
