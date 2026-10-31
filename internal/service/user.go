// Package service 定义业务逻辑与事务边界（handler 只做协议转换，业务在此层）。
package service

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/cool-wx/sys-iam-go/internal/model/dto"
	"github.com/cool-wx/sys-iam-go/internal/model/po"
	"github.com/cool-wx/sys-iam-go/internal/repository"
	"github.com/cool-wx/sys-iam-go/pkg/errcode"
	"github.com/cool-wx/sys-iam-go/pkg/paging"
)

// UserService 用户域服务。
type UserService struct {
	repo *repository.UserRepo
}

// NewUserService 构造。
func NewUserService(repo *repository.UserRepo) *UserService {
	return &UserService{repo: repo}
}

// Create 创建用户：DTO→PO（服务端默认值 Status=1/FirstLogin=true/Builtin=false，
// 源 User.create() 语义；password 按 D-01 边界原样入库，哈希化属 Phase 2 User 域规则），
// 落库后 PO→DTO 返回。审计字段（双时间/四操作人/ID）由 PO 钩子服务端填充，
// 不接受请求体任何同名客户端字段（ENG-02）。
func (s *UserService) Create(ctx context.Context, req *dto.UserCreateReq) (*dto.UserResp, error) {
	u := &po.UserPO{
		Username: req.Username,
		Password: req.Password,
		RealName: req.RealName,
		Phone:    req.Phone,
		Email:    req.Email,
		DeptName: req.DeptName,
	}
	status := int8(1)
	u.Status = &status
	firstLogin := true
	u.FirstLogin = &firstLogin
	builtin := false
	u.Builtin = &builtin
	if req.Gender != nil {
		g := int8(*req.Gender)
		u.Gender = &g
	}
	if req.DeptID != nil {
		d := int64(*req.DeptID)
		u.DeptID = &d
	}

	if err := s.repo.Create(ctx, u); err != nil {
		// TranslateError 开启后 MySQL 1062 → gorm.ErrDuplicatedKey（P4）
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return nil, errcode.New(errcode.CodeInvalidParam, "用户名已存在")
		}
		return nil, err
	}
	return dto.ToUserResp(u), nil
}

// Update 更新用户（部分更新，ENG-05）：遍历 UpdateUserReq 全指针字段，仅非 nil
// 者进更新 map（nil=未提供不写库；空串=显式清空照写），列名对齐 DDL snake_case；
// 请求体之外的字段不可能进 map（白名单天然收敛，T-1-10）。落库后回读返回最新契约。
// 审计（update_time/update_by）由 repo.Patch 显式并入，不接受客户端同名键（ENG-02）。
func (s *UserService) Update(ctx context.Context, id int64, req *dto.UpdateUserReq) (*dto.UserResp, error) {
	cols := map[string]any{}
	if req.RealName != nil {
		cols["real_name"] = *req.RealName
	}
	if req.Phone != nil {
		cols["phone"] = *req.Phone
	}
	if req.Email != nil {
		cols["email"] = *req.Email
	}
	if req.Avatar != nil {
		cols["avatar"] = *req.Avatar
	}
	if req.DeptID != nil {
		cols["dept_id"] = int64(*req.DeptID)
	}
	if req.DeptName != nil {
		cols["dept_name"] = *req.DeptName
	}
	if req.Gender != nil {
		cols["gender"] = int8(*req.Gender)
	}
	if req.Builtin != nil {
		cols["builtin"] = *req.Builtin
	}
	if req.Remark != nil {
		cols["remark"] = *req.Remark
	}
	if err := s.repo.Patch(ctx, id, cols); err != nil {
		return nil, err
	}
	return s.Get(ctx, id)
}

// List 用户列表：repo 分页查询 → PO→DTO 转换 → paging.PageData envelope
// （排序与钳位由 repo/paging 承担，P15）。
func (s *UserService) List(ctx context.Context, page, size int) (*paging.PageData[dto.UserResp], error) {
	users, total, err := s.repo.List(ctx, page, size)
	if err != nil {
		return nil, err
	}
	items := make([]dto.UserResp, 0, len(users))
	for i := range users {
		items = append(items, *dto.ToUserResp(&users[i]))
	}
	return &paging.PageData[dto.UserResp]{Items: items, Total: total, Page: page, Size: size}, nil
}

// Get 用户详情：按主键查询后 PO→DTO（未命中 404/10300）。
func (s *UserService) Get(ctx context.Context, id int64) (*dto.UserResp, error) {
	u, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return dto.ToUserResp(u), nil
}
