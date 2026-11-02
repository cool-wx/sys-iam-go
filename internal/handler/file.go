// Package handler 文件域 HTTP handler（薄层三段式；Upload 为 multipart 新形态、
// Download 为流式出站新形态——FILE-01/02/03）。
package handler

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/cool-wx/sys-iam-go/internal/service"
	"github.com/cool-wx/sys-iam-go/pkg/errcode"
	"github.com/cool-wx/sys-iam-go/pkg/paging"
	"github.com/cool-wx/sys-iam-go/pkg/resp"
)

// FileHandler 文件域 HTTP handler。
type FileHandler struct {
	svc *service.FileService
}

// NewFileHandler 构造。
func NewFileHandler(svc *service.FileService) *FileHandler {
	return &FileHandler{svc: svc}
}

// Upload 处理 POST /api/v1/files/upload（multipart；BodyLimitWithMB(20) 已在路由
// 层封顶）：FormFile("file") 取 multipart FileHeader + PostForm("category_id")
// 可选 → service 上传链（判重五语义）→ envelope。
func (h *FileHandler) Upload(c *gin.Context) {
	fh, err := c.FormFile("file")
	if err != nil {
		resp.Fail(c, errcode.New(errcode.CodeInvalidParam, "缺少上传文件字段 file"))
		return
	}
	data, err := h.svc.Upload(c.Request.Context(), fh, c.PostForm("category_id"))
	if err != nil {
		resp.Fail(c, err)
		return
	}
	resp.OK(c, data)
}

// Download 处理 GET /api/v1/files/download/:id（FILE-02 流式，全链不整载）：
// Content-Length 由 DataFromReader 按真实字节数写出；Content-Disposition 手写
//
//	attachment; filename*=UTF-8'' + url.PathEscape
//
// ——RFC 5987 形态承载中文与特殊字符，PathEscape 对 CR/LF 与控制字符百分号编码，
// 头注入面天然消除（T-4-03），恒 attachment 不以同源 inline 渲染（T-4-05 缓解面）。
func (h *FileHandler) Download(c *gin.Context) {
	id, ok := parseID(c, "无效的文件 ID")
	if !ok {
		return
	}
	res, err := h.svc.Download(c.Request.Context(), id)
	if err != nil {
		resp.Fail(c, err)
		return
	}
	defer res.Reader.Close()

	fileName := "file"
	if res.Row.FileName != nil && *res.Row.FileName != "" {
		fileName = *res.Row.FileName
	}
	extraHeaders := map[string]string{
		"Content-Disposition": `attachment; filename*=UTF-8''` + url.PathEscape(fileName),
	}
	// DataFromReader 内部 io.Copy 流式写出（恒不先全量读内存——FILE-02 RSS 验收；
	// 存储读中断的半截响应已由 GetStream 的 StatObject 前置把 404 挡在响应前）
	c.DataFromReader(http.StatusOK, res.Size, res.ContentType, res.Reader, extraHeaders)
}

// List 处理 GET /api/v1/files：分页（paging.Parse 1/15 钳位）+ category_id 可选
// 过滤 → envelope。
func (h *FileHandler) List(c *gin.Context) {
	page, size := paging.Parse(c.Query("page"), c.Query("size"))
	var categoryID *int64
	if raw := c.Query("category_id"); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			resp.Fail(c, errcode.New(errcode.CodeInvalidParam, "无效的文件分类 ID"))
			return
		}
		categoryID = &id
	}
	data, err := h.svc.List(c.Request.Context(), page, size, categoryID)
	if err != nil {
		resp.Fail(c, err)
		return
	}
	resp.OK(c, data)
}

// Delete 处理 DELETE /api/v1/files/:id：解析 :id → service（存储宽容删除）→ envelope。
func (h *FileHandler) Delete(c *gin.Context) {
	id, ok := parseID(c, "无效的文件 ID")
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), id); err != nil {
		resp.Fail(c, err)
		return
	}
	resp.OK(c, nil)
}
