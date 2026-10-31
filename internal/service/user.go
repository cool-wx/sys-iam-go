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
