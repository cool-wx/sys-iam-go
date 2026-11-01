// Package service 字典域业务逻辑：类型 CRUD（uk_dict_type 1062 翻译）+ 数据 CRUD
// （类型存在性前置校验 + dict_type_name 服务端冗余写，DICT-01/02/03）。
package service

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/cool-wx/sys-iam-go/internal/model/dto"
	"github.com/cool-wx/sys-iam-go/internal/model/po"
	"github.com/cool-wx/sys-iam-go/internal/repository"
	"github.com/cool-wx/sys-iam-go/pkg/errcode"
	"github.com/cool-wx/sys-iam-go/pkg/paging"
)

// DictService 字典域服务（类型 + 数据合并一个 service——04-RESEARCH Implementation
// Guidance 建议形态；数据侧依赖类型 repo 做存在性校验与冗余名读取）。
type DictService struct {
	typeRepo *repository.DictTypeRepo
	dataRepo *repository.DictDataRepo
}

// NewDictService 构造。
func NewDictService(typeRepo *repository.DictTypeRepo, dataRepo *repository.DictDataRepo) *DictService {
	return &DictService{typeRepo: typeRepo, dataRepo: dataRepo}
}

// ---- 字典类型侧（DICT-01）----

// CreateDictType 创建字典类型：DTO→PO（status 服务端默认 1 正常——对齐 DDL default
// 与源 MP null-skip 语义）；uk_dict_type 冲突（TranslateError 开启后 MySQL 1062 →
// gorm.ErrDuplicatedKey，P4）译为 400/10400「字典类型已存在」——Phase 2 惯例代码
// 实证为 400/10400 非 409（role.go 先例）。审计字段由 PO 钩子服务端填充，不接受
// 请求体任何同名客户端字段（ENG-02）。
func (s *DictService) CreateDictType(ctx context.Context, req *dto.DictTypeCreateReq) (*dto.DictTypeResp, error) {
	m := &po.DictTypePO{
		DictName: *req.DictName,
		DictType: *req.DictType,
		Remark:   req.Remark,
	}
	status := int8(1)
	m.Status = &status
	if err := s.typeRepo.Create(ctx, m); err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return nil, errcode.New(errcode.CodeInvalidParam, "字典类型已存在")
		}
		return nil, err
	}
	return dto.ToDictTypeResp(m), nil
}

// UpdateDictType 更新字典类型（部分更新，ENG-05）：白名单 dict_name/dict_type/
// status/remark（nil=不写库；空串=显式清空照写）。落库后回读返回最新契约。
// 审计由 repo.Patch 显式并入（ENG-02）。
func (s *DictService) UpdateDictType(ctx context.Context, id int64, req *dto.DictTypeUpdateReq) (*dto.DictTypeResp, error) {
	cols := map[string]any{}
	if req.DictName != nil {
		cols["dict_name"] = *req.DictName
	}
	if req.DictType != nil {
		cols["dict_type"] = *req.DictType
	}
	if req.Status != nil {
		cols["status"] = int8(*req.Status)
	}
	if req.Remark != nil {
		cols["remark"] = *req.Remark
	}
	if err := s.typeRepo.Patch(ctx, id, cols); err != nil {
		if errors.Is(err, errcode.ErrNotFound) {
			return nil, errcode.New(errcode.CodeNotFound, fmt.Sprintf("字典类型不存在: %d", id))
		}
		return nil, err
	}
	return s.GetDictType(ctx, id)
}

// GetDictType 字典类型详情：未命中译为 404/10300「字典类型不存在: {id}」
// （源消息原文 DictApplicationService:49；code/HTTP 契约与 Phase 1 一致）。
func (s *DictService) GetDictType(ctx context.Context, id int64) (*dto.DictTypeResp, error) {
	m, err := s.typeRepo.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, errcode.ErrNotFound) {
			return nil, errcode.New(errcode.CodeNotFound, fmt.Sprintf("字典类型不存在: %d", id))
		}
		return nil, err
	}
	return dto.ToDictTypeResp(m), nil
}

// ListDictTypes 字典类型分页列表（无 dataScope——D-03 拍板零应用）。
func (s *DictService) ListDictTypes(ctx context.Context, page, size int) (*paging.PageData[dto.DictTypeResp], error) {
	items, total, err := s.typeRepo.List(ctx, page, size)
	if err != nil {
		return nil, err
	}
	out := make([]dto.DictTypeResp, 0, len(items))
	for i := range items {
		out = append(out, *dto.ToDictTypeResp(&items[i]))
	}
	return &paging.PageData[dto.DictTypeResp]{Items: out, Total: total, Page: page, Size: size}, nil
}

// DeleteDictType 删除字典类型：直接物理删。与源一致**不加字典数据引用检查**
// （源缺口按源保留：deleteDictType 直接删——删除类型后其下 dict_data 成为孤儿）。
func (s *DictService) DeleteDictType(ctx context.Context, id int64) error {
	return s.typeRepo.Delete(ctx, id)
}

// ---- 字典数据侧（DICT-02/03）----

// CreateDictData 创建字典数据：第一步先校验类型存在（缺失 → 404/10300
// 「字典类型不存在: {typeId}」源消息原文 DictApplicationService:87）；命中后把
// 类型的 dict_name 冗余写入行内 dict_type_name 列（源 addDictData 冗余写行为，
// 服务端查库值不信请求体，T-1-10）。
func (s *DictService) CreateDictData(ctx context.Context, req *dto.DictDataCreateReq) (*dto.DictDataResp, error) {
	typeID := int64(*req.DictTypeID)
	dt, err := s.resolveDictType(ctx, typeID)
	if err != nil {
		return nil, err
	}
	m := &po.DictDataPO{
		DictTypeID:   typeID,
		DictTypeName: &dt.DictName, // 服务端查库冗余（DICT-02）
		Label:        req.Label,
		Value:        req.Value,
		Sort:         req.Sort,
		Remark:       req.Remark,
	}
	if err := s.dataRepo.Create(ctx, m); err != nil {
		return nil, err
	}
	return dto.ToDictDataResp(m), nil
}

// UpdateDictData 更新字典数据（部分更新，ENG-05）：dictTypeId 显式传入时联动
// 类型校验 + dictTypeName 重冗余（源 editDictData:102 每次编辑执行校验语义）；
// 不显式传则不动类型列。落库后回读返回最新契约。
func (s *DictService) UpdateDictData(ctx context.Context, id int64, req *dto.DictDataUpdateReq) (*dto.DictDataResp, error) {
	cols := map[string]any{}
	if req.DictTypeID != nil {
		typeID := int64(*req.DictTypeID)
		dt, err := s.resolveDictType(ctx, typeID)
		if err != nil {
			return nil, err
		}
		cols["dict_type_id"] = typeID
		cols["dict_type_name"] = dt.DictName
	}
	if req.Label != nil {
		cols["label"] = *req.Label
	}
	if req.Value != nil {
		cols["value"] = *req.Value
	}
	if req.Sort != nil {
		cols["sort"] = *req.Sort
	}
	if req.Remark != nil {
		cols["remark"] = *req.Remark
	}
	if err := s.dataRepo.Patch(ctx, id, cols); err != nil {
		if errors.Is(err, errcode.ErrNotFound) {
			return nil, errcode.New(errcode.CodeNotFound, fmt.Sprintf("字典数据不存在: %d", id))
		}
		return nil, err
	}
	return s.GetDictData(ctx, id)
}

// GetDictData 字典数据详情：未命中译为 404/10300。
func (s *DictService) GetDictData(ctx context.Context, id int64) (*dto.DictDataResp, error) {
	m, err := s.dataRepo.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, errcode.ErrNotFound) {
			return nil, errcode.New(errcode.CodeNotFound, fmt.Sprintf("字典数据不存在: %d", id))
		}
		return nil, err
	}
	return dto.ToDictDataResp(m), nil
}

// ListDictDatas 字典数据分页列表；dictTypeID 非零时按类型过滤（DICT-03 可选分支，
// 源 list?dictTypeId= 语义）。
func (s *DictService) ListDictDatas(ctx context.Context, page, size int, dictTypeID int64) (*paging.PageData[dto.DictDataResp], error) {
	items, total, err := s.dataRepo.List(ctx, page, size, dictTypeID)
	if err != nil {
		return nil, err
	}
	out := make([]dto.DictDataResp, 0, len(items))
	for i := range items {
		out = append(out, *dto.ToDictDataResp(&items[i]))
	}
	return &paging.PageData[dto.DictDataResp]{Items: out, Total: total, Page: page, Size: size}, nil
}

// DeleteDictData 删除字典数据：直接物理删（源 deleteDictData 语义）。
func (s *DictService) DeleteDictData(ctx context.Context, id int64) error {
	return s.dataRepo.Delete(ctx, id)
}

// resolveDictType 类型存在性校验（源 resolveDictType 语义）：不存在报
// 404/10300「字典类型不存在: {typeId}」源消息原文（DictApplicationService:87/102）。
func (s *DictService) resolveDictType(ctx context.Context, typeID int64) (*po.DictTypePO, error) {
	dt, err := s.typeRepo.FindByID(ctx, typeID)
	if err != nil {
		if errors.Is(err, errcode.ErrNotFound) {
			return nil, errcode.New(errcode.CodeNotFound, fmt.Sprintf("字典类型不存在: %d", typeID))
		}
		return nil, err
	}
	return dt, nil
}
