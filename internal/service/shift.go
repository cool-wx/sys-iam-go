// Package service 排班域业务逻辑：CRUD（全量查询无分页——源行为直译；
// start_time/end_time HH:MM:SS 字符串直传，无 1062 分支，sys_shift
// 无唯一索引）。
package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/cool-wx/sys-iam-go/internal/model/dto"
	"github.com/cool-wx/sys-iam-go/internal/model/po"
	"github.com/cool-wx/sys-iam-go/internal/repository"
	"github.com/cool-wx/sys-iam-go/pkg/errcode"
)

// ShiftService 排班域服务。
type ShiftService struct {
	repo *repository.ShiftRepo
}

// NewShiftService 构造。
func NewShiftService(repo *repository.ShiftRepo) *ShiftService {
	return &ShiftService{repo: repo}
}

// Create 创建排班：status 不收客户端值，服务端默认 1 正常（对齐 DDL default 与
// 源 MP null-skip 语义；对齐 dict/fileCategory 域 Create 形态）。审计字段
// 由 PO 钩子服务端填充。
func (s *ShiftService) Create(ctx context.Context, req *dto.ShiftCreateReq) (*dto.ShiftResp, error) {
	m := &po.ShiftPO{
		ShiftCode: req.ShiftCode,
		ShiftName: req.ShiftName,
		StartTime: req.StartTime,
		EndTime:   req.EndTime,
		Remark:    req.Remark,
	}
	status := int8(1)
	m.Status = &status
	if err := s.repo.Create(ctx, m); err != nil {
		return nil, err
	}
	return dto.ToShiftResp(m), nil
}

// Update 更新排班（部分更新）：白名单 shift_code/shift_name/start_time/
// end_time/status/remark（nil=不写库；空串=显式清空照写）。落库后回读返回。
func (s *ShiftService) Update(ctx context.Context, id int64, req *dto.ShiftUpdateReq) (*dto.ShiftResp, error) {
	cols := map[string]any{}
	if req.ShiftCode != nil {
		cols["shift_code"] = *req.ShiftCode
	}
	if req.ShiftName != nil {
		cols["shift_name"] = *req.ShiftName
	}
	if req.StartTime != nil {
		cols["start_time"] = *req.StartTime
	}
	if req.EndTime != nil {
		cols["end_time"] = *req.EndTime
	}
	if req.Status != nil {
		// UsageStatus 严格解析：未知值已在绑定层 400/10401，此处恒合法
		cols["status"] = int8(*req.Status)
	}
	if req.Remark != nil {
		cols["remark"] = *req.Remark
	}
	if err := s.repo.Patch(ctx, id, cols); err != nil {
		if errors.Is(err, errcode.ErrNotFound) {
			return nil, errcode.New(errcode.CodeNotFound, fmt.Sprintf("排班不存在: %d", id))
		}
		return nil, err
	}
	return s.Get(ctx, id)
}

// Get 排班详情：未命中译为 404/10300「排班不存在: {id}」。
func (s *ShiftService) Get(ctx context.Context, id int64) (*dto.ShiftResp, error) {
	m, err := s.repo.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, errcode.ErrNotFound) {
			return nil, errcode.New(errcode.CodeNotFound, fmt.Sprintf("排班不存在: %d", id))
		}
		return nil, err
	}
	return dto.ToShiftResp(m), nil
}

// List 排班全量列表——源 list 无分页行为直译，勿统一为分页
// （repo ORDER BY shift_code ASC, id ASC）。
func (s *ShiftService) List(ctx context.Context) ([]*dto.ShiftResp, error) {
	items, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*dto.ShiftResp, 0, len(items))
	for i := range items {
		out = append(out, dto.ToShiftResp(&items[i]))
	}
	return out, nil
}

// Delete 删除排班：直接物理删（源 deleteShift 语义）。
func (s *ShiftService) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}
