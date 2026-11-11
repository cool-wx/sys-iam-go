// Package service 故障域业务逻辑：CRUD + 分页列表（code 索引列 eq
// 过滤）+ 服务端默认值 bit_index=-1/type=3（GORM nil 指针显式写 NULL 会绕过 DB
// 列默认——默认值语义由本层承担，不依赖 DDL DEFAULT）。Excel 三键 upsert 导入
// 见 fault_import.go。无 builtin 守卫（fault 域无 builtin 概念）；
// 无唯一索引无 1062 分支。
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
	"github.com/cool-wx/sys-iam-go/pkg/types"
)

// FaultService 故障域服务。db 供 Excel 导入显式事务（SkipDefaultTransaction 下
// 唯一事务边界）。
type FaultService struct {
	db   *gorm.DB
	repo *repository.FaultInfoRepo
}

// NewFaultService 构造。
func NewFaultService(db *gorm.DB, repo *repository.FaultInfoRepo) *FaultService {
	return &FaultService{db: db, repo: repo}
}

// Create 创建故障信息：bitIndex 未传服务端默认 -1、type 未传默认 3(UNKNOWN)
// （字段默认值语义——DDL DEFAULT -1/3 仅为兜底，GORM nil 指针显式写 NULL 会
// 绕过列默认，故服务端承担）。审计字段由 PO 钩子填充。
func (s *FaultService) Create(ctx context.Context, req *dto.FaultCreateReq) (*dto.FaultResp, error) {
	m := &po.FaultInfoPO{
		Code:     req.Code,
		Name:     req.Name,
		Detail:   req.Detail,
		Reason:   req.Reason,
		Location: req.Location,
		Fix:      req.Fix,
		Remark:   req.Remark,
	}
	if req.BitIndex != nil {
		m.BitIndex = req.BitIndex
	} else {
		bi := -1
		m.BitIndex = &bi
	}
	if req.Type != nil {
		m.Type = (*int8)(req.Type)
	} else {
		ft := int8(types.FaultTypeUnknown)
		m.Type = &ft
	}
	if req.DictDataID != nil {
		dd := int64(*req.DictDataID)
		m.DictDataID = &dd
	}
	if err := s.repo.Create(ctx, m); err != nil {
		return nil, err
	}
	return dto.ToFaultResp(m), nil
}

// Update 更新故障信息（部分更新）：白名单 code/name/bit_index/type/
// dict_data_id/detail/reason/location/fix/remark（nil=不写库；空串=显式清空
// 照写）。落库后回读返回。
func (s *FaultService) Update(ctx context.Context, id int64, req *dto.FaultUpdateReq) (*dto.FaultResp, error) {
	cols := map[string]any{}
	if req.Code != nil {
		cols["code"] = *req.Code
	}
	if req.Name != nil {
		cols["name"] = *req.Name
	}
	if req.BitIndex != nil {
		cols["bit_index"] = *req.BitIndex
	}
	if req.Type != nil {
		cols["type"] = int8(*req.Type)
	}
	if req.DictDataID != nil {
		cols["dict_data_id"] = int64(*req.DictDataID)
	}
	if req.Detail != nil {
		cols["detail"] = *req.Detail
	}
	if req.Reason != nil {
		cols["reason"] = *req.Reason
	}
	if req.Location != nil {
		cols["location"] = *req.Location
	}
	if req.Fix != nil {
		cols["fix"] = *req.Fix
	}
	if req.Remark != nil {
		cols["remark"] = *req.Remark
	}
	if err := s.repo.Patch(ctx, id, cols); err != nil {
		if errors.Is(err, errcode.ErrNotFound) {
			return nil, errcode.New(errcode.CodeNotFound, fmt.Sprintf("故障信息不存在: %d", id))
		}
		return nil, err
	}
	return s.Get(ctx, id)
}

// Get 故障详情：未命中译为 404/10300「故障信息不存在: {id}」（域消息惯例）。
func (s *FaultService) Get(ctx context.Context, id int64) (*dto.FaultResp, error) {
	m, err := s.repo.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, errcode.ErrNotFound) {
			return nil, errcode.New(errcode.CodeNotFound, fmt.Sprintf("故障信息不存在: %d", id))
		}
		return nil, err
	}
	return dto.ToFaultResp(m), nil
}

// List 分页条件查询：code 非空时 idx_code 索引列 eq 直传过滤（无 like，
// 无其他特殊条件）；envelope 为 paging.PageData 契约。
func (s *FaultService) List(ctx context.Context, page, size int, code string) (*paging.PageData[dto.FaultResp], error) {
	faults, total, err := s.repo.List(ctx, page, size, code)
	if err != nil {
		return nil, err
	}
	items := make([]dto.FaultResp, 0, len(faults))
	for i := range faults {
		items = append(items, *dto.ToFaultResp(&faults[i]))
	}
	return &paging.PageData[dto.FaultResp]{Items: items, Total: total, Page: page, Size: size}, nil
}

// Delete 删除故障信息：直接物理删（无存在性前置校验）。
func (s *FaultService) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}
