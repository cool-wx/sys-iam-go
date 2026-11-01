// Package handler 字典数据域 HTTP handler（薄层三段式；list 支持 dict_type_id
// 可选过滤——DICT-03 源 list?dictTypeId= 语义）。
package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/cool-wx/sys-iam-go/internal/model/dto"
	"github.com/cool-wx/sys-iam-go/internal/service"
	"github.com/cool-wx/sys-iam-go/pkg/errcode"
	"github.com/cool-wx/sys-iam-go/pkg/paging"
	"github.com/cool-wx/sys-iam-go/pkg/resp"
)

// DictDataHandler 字典数据域 HTTP handler。
type DictDataHandler struct {
	svc *service.DictService
}

// NewDictDataHandler 构造。
func NewDictDataHandler(svc *service.DictService) *DictDataHandler {
	return &DictDataHandler{svc: svc}
}

// Create 处理 POST /api/v1/dict-datas：绑定校验 → service（类型存在性校验 +
// dictTypeName 冗余写）→ envelope 出口。
func (h *DictDataHandler) Create(c *gin.Context) {
	var req dto.DictDataCreateReq
	if !bindJSON(c, &req) {
		return
	}
	data, err := h.svc.CreateDictData(c.Request.Context(), &req)
	if err != nil {
		resp.Fail(c, err)
		return
	}
	resp.OK(c, data)
}

// Update 处理 PUT /api/v1/dict-datas/:id：解析 :id → 绑定指针 DTO → service → envelope。
func (h *DictDataHandler) Update(c *gin.Context) {
	id, ok := parseID(c, "无效的字典数据 ID")
	if !ok {
		return
	}
	var req dto.DictDataUpdateReq
	if !bindJSON(c, &req) {
		return
	}
	data, err := h.svc.UpdateDictData(c.Request.Context(), id, &req)
	if err != nil {
		resp.Fail(c, err)
		return
	}
	resp.OK(c, data)
}

// Get 处理 GET /api/v1/dict-datas/:id：解析 :id → service → envelope。
func (h *DictDataHandler) Get(c *gin.Context) {
	id, ok := parseID(c, "无效的字典数据 ID")
	if !ok {
		return
	}
	data, err := h.svc.GetDictData(c.Request.Context(), id)
	if err != nil {
		resp.Fail(c, err)
		return
	}
	resp.OK(c, data)
}

// List 处理 GET /api/v1/dict-datas：分页列表；dict_type_id 查询参数非空时可选
// 过滤（DICT-03），非法值 400/10400（query 参数形态的 parseID 变体）。
func (h *DictDataHandler) List(c *gin.Context) {
	page, size := paging.Parse(c.Query("page"), c.Query("size"))
	var dictTypeID int64
	if raw := c.Query("dict_type_id"); raw != "" {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			resp.Fail(c, errcode.New(errcode.CodeInvalidParam, "无效的字典类型 ID"))
			return
		}
		dictTypeID = v
	}
	data, err := h.svc.ListDictDatas(c.Request.Context(), page, size, dictTypeID)
	if err != nil {
		resp.Fail(c, err)
		return
	}
	resp.OK(c, data)
}

// Delete 处理 DELETE /api/v1/dict-datas/:id：解析 :id → service（物理删）→ envelope。
func (h *DictDataHandler) Delete(c *gin.Context) {
	id, ok := parseID(c, "无效的字典数据 ID")
	if !ok {
		return
	}
	if err := h.svc.DeleteDictData(c.Request.Context(), id); err != nil {
		resp.Fail(c, err)
		return
	}
	resp.OK(c, nil)
}
