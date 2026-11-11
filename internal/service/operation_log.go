// Package service 操作日志查询域业务逻辑（分页检索 1/10 特例 + 三 like
// 模糊）。写侧自动记录由 middleware.Oplog 承担，service 只做读侧装配。
package service

import (
	"context"

	"github.com/cool-wx/sys-iam-go/internal/model/dto"
	"github.com/cool-wx/sys-iam-go/internal/repository"
	"github.com/cool-wx/sys-iam-go/pkg/paging"
)

// OperationLogService 操作日志查询服务。
type OperationLogService struct {
	repo *repository.OperationLogRepo
}

// NewOperationLogService 构造。
func NewOperationLogService(repo *repository.OperationLogRepo) *OperationLogService {
	return &OperationLogService{repo: repo}
}

// List 操作日志分页列表（无 dataScope）。module/operation/
// realName 为 like 模糊条件（空串=不过滤）。分页参数已由 handler 经
// paging.ParseWithDefaults 钳位（本域缺省 1/10 例外；畸形输入统一归
// 缺省 10，与全局 1/15 的差异仅影响畸形请求纠偏值）。
func (s *OperationLogService) List(ctx context.Context, page, size int, module, operation, realName string) (*paging.PageData[dto.OlogResp], error) {
	items, total, err := s.repo.List(ctx, page, size, repository.OlogListFilter{
		Module:    module,
		Operation: operation,
		RealName:  realName,
	})
	if err != nil {
		return nil, err
	}
	out := make([]dto.OlogResp, 0, len(items))
	for i := range items {
		out = append(out, *dto.ToOlogResp(&items[i]))
	}
	return &paging.PageData[dto.OlogResp]{Items: out, Total: total, Page: page, Size: size}, nil
}
