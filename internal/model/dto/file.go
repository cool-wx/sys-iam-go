// Package dto File 域响应/请求契约：文件分类（树形）+ 文件信息
// （分页列表 + 上传/下载/删除链载体）。全部键无 omitempty（未赋值字段以 null 键输出）；
// 不得定义任何审计字段键（审计由 PO 钩子服务端填充）。
package dto

import (
	"github.com/cool-wx/sys-iam-go/internal/model/po"
	"github.com/cool-wx/sys-iam-go/pkg/ids"
	"github.com/cool-wx/sys-iam-go/pkg/types"
)

// ---- 文件分类侧（与部门域同构）----

// FileCategoryCreateReq 创建文件分类请求。categoryName 必填由 binding 校验拦截；
// parentId 传 0 与不传同义（根）；status 服务端默认 1 正常（对齐 DDL default）。
type FileCategoryCreateReq struct {
	CategoryName *string `json:"categoryName" binding:"required"`
	ParentID     *ids.ID `json:"parentId"`
	CategoryType *int    `json:"categoryType"`
	Sort         *int    `json:"sort"`
	Icon         *string `json:"icon"`
	Remark       *string `json:"remark"`
}

// FileCategoryUpdateReq 更新文件分类请求：全指针字段，nil=未提供（不写库），
// 空串=显式清空。parentId 显式传入时联动父校验 + parentName 重冗余（服务端查库装配）；
// 白名单收敛于本结构键集。
type FileCategoryUpdateReq struct {
	CategoryName *string            `json:"categoryName"`
	ParentID     *ids.ID            `json:"parentId"`
	CategoryType *int               `json:"categoryType"`
	Sort         *int               `json:"sort"`
	Icon         *string            `json:"icon"`
	Status       *types.UsageStatus `json:"status"`
	Remark       *string            `json:"remark"`
}

// FileCategoryResp 文件分类响应契约。
type FileCategoryResp struct {
	ID           ids.ID               `json:"id"`
	ParentID     *ids.ID              `json:"parentId"`
	ParentName   *string              `json:"parentName"`
	CategoryName *string              `json:"categoryName"`
	CategoryType *int                 `json:"categoryType"`
	Sort         *int                 `json:"sort"`
	Icon         *string              `json:"icon"`
	Status       *types.UsageStatus   `json:"status"`
	CreateByID   *ids.ID              `json:"createById"`
	CreateByName *string              `json:"createByName"`
	UpdateByID   *ids.ID              `json:"updateById"`
	UpdateByName *string              `json:"updateByName"`
	CreateTime   *types.LocalDateTime `json:"createTime"`
	UpdateTime   *types.LocalDateTime `json:"updateTime"`
	Remark       *string              `json:"remark"`
}

// FileCategoryTreeResp 树节点：FileCategoryResp 内嵌 + children 递归嵌套。
// Children 恒非 nil（叶子经 tree.Build attach 空切片归一化）——JSON 出口 [] 非 null。
type FileCategoryTreeResp struct {
	FileCategoryResp
	Children []*FileCategoryTreeResp `json:"children"`
}

// ToFileCategoryResp PO→DTO 手写转换（枚举经 FromInt 兜底；int64 ID→契约 ID 类型）。
func ToFileCategoryResp(m *po.FileCategoryPO) *FileCategoryResp {
	r := &FileCategoryResp{
		ID:           ids.ID(m.ID),
		ParentName:   m.ParentName,
		CategoryName: m.CategoryName,
		CategoryType: m.CategoryType,
		Sort:         m.Sort,
		Icon:         m.Icon,
		CreateByName: m.CreateByName,
		UpdateByName: m.UpdateByName,
		Remark:       m.Remark,
	}
	if m.ParentID != nil {
		id := ids.ID(*m.ParentID)
		r.ParentID = &id
	}
	if m.Status != nil {
		s := types.UsageStatusFromInt(*m.Status)
		r.Status = &s
	}
	if m.CreateByID != nil {
		id := ids.ID(*m.CreateByID)
		r.CreateByID = &id
	}
	if m.UpdateByID != nil {
		id := ids.ID(*m.UpdateByID)
		r.UpdateByID = &id
	}
	if m.CreateTime != nil {
		t := types.LocalDateTime(*m.CreateTime)
		r.CreateTime = &t
	}
	if m.UpdateTime != nil {
		t := types.LocalDateTime(*m.UpdateTime)
		r.UpdateTime = &t
	}
	return r
}

// ToFileCategoryTreeResp PO→树节点转换：Children 预置空切片（叶子出口 [] 非 null；
// 非叶子由 tree.Build 的 attach 回调替换为真实子树）。
func ToFileCategoryTreeResp(m *po.FileCategoryPO) *FileCategoryTreeResp {
	return &FileCategoryTreeResp{
		FileCategoryResp: *ToFileCategoryResp(m),
		Children:         []*FileCategoryTreeResp{},
	}
}

// ---- 文件信息侧----

// FileInfoResp 文件信息响应契约（列表项 + 上传返回体）。file_path 为服务端生成
// 的存储对象键（uuid_原名），下载寻址恒经 DB 内 file_path（非客户端参数）。
type FileInfoResp struct {
	ID           ids.ID               `json:"id"`
	CategoryID   *ids.ID              `json:"categoryId"`
	CategoryName *string              `json:"categoryName"`
	FileName     *string              `json:"fileName"`
	FilePath     *string              `json:"filePath"`
	FileType     *string              `json:"fileType"`
	FileSize     *int64               `json:"fileSize"`
	FileHash     *string              `json:"fileHash"`
	Description  *string              `json:"description"`
	CreateByID   *ids.ID              `json:"createById"`
	CreateByName *string              `json:"createByName"`
	UpdateByID   *ids.ID              `json:"updateById"`
	UpdateByName *string              `json:"updateByName"`
	CreateTime   *types.LocalDateTime `json:"createTime"`
	UpdateTime   *types.LocalDateTime `json:"updateTime"`
	Remark       *string              `json:"remark"`
}

// ToFileInfoResp PO→DTO 手写转换（int64 ID→契约 ID 类型）。
func ToFileInfoResp(m *po.FileInfoPO) *FileInfoResp {
	r := &FileInfoResp{
		ID:           ids.ID(m.ID),
		CategoryName: m.CategoryName,
		FileName:     m.FileName,
		FilePath:     m.FilePath,
		FileType:     m.FileType,
		FileSize:     m.FileSize,
		FileHash:     m.FileHash,
		Description:  m.Description,
		CreateByName: m.CreateByName,
		UpdateByName: m.UpdateByName,
		Remark:       m.Remark,
	}
	if m.CategoryID != nil {
		id := ids.ID(*m.CategoryID)
		r.CategoryID = &id
	}
	if m.CreateByID != nil {
		id := ids.ID(*m.CreateByID)
		r.CreateByID = &id
	}
	if m.UpdateByID != nil {
		id := ids.ID(*m.UpdateByID)
		r.UpdateByID = &id
	}
	if m.CreateTime != nil {
		t := types.LocalDateTime(*m.CreateTime)
		r.CreateTime = &t
	}
	if m.UpdateTime != nil {
		t := types.LocalDateTime(*m.UpdateTime)
		r.UpdateTime = &t
	}
	return r
}
