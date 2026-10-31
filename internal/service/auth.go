// 认证域服务：登录编排（等价源 AuthApplicationService 六步链）与登出。
package service

import (
	"context"

	"golang.org/x/crypto/bcrypt"

	"github.com/cool-wx/sys-iam-go/internal/auth"
	"github.com/cool-wx/sys-iam-go/internal/model/dto"
	"github.com/cool-wx/sys-iam-go/internal/model/po"
	"github.com/cool-wx/sys-iam-go/internal/repository"
	"github.com/cool-wx/sys-iam-go/pkg/errcode"
)

// AuthService 认证服务：用户/角色/会话三方编排。
type AuthService struct {
	userRepo     *repository.UserRepo
	relationRepo *repository.RelationRepo
	sm           *auth.SessionManager
	// dummyHash 恒时防枚举（T-2-01）：用户不存在路径对它执行一次真实 bcrypt 比较，
	// 使"用户不存在"与"密码错误"两路径耗时量级一致（bcrypt cost 10 ~100ms），
	// 不可由响应时间差枚举用户名。
	dummyHash []byte
}

// NewAuthService 构造；dummyHash 在此一次生成常量持有（不落库不出网）。
func NewAuthService(userRepo *repository.UserRepo, relationRepo *repository.RelationRepo, sm *auth.SessionManager) *AuthService {
	dummyHash, _ := bcrypt.GenerateFromPassword([]byte("sys-iam-go:anti-enumeration-dummy"), bcrypt.DefaultCost)
	return &AuthService{
		userRepo:     userRepo,
		relationRepo: relationRepo,
		sm:           sm,
		dummyHash:    dummyHash,
	}
}

// Login 用户名密码登录（顺序与消息照源 AuthApplicationService.java:52-60，D-02）：
// ①查用户（不存在 → 对 dummyHash 恒时比较后统一 10501）②状态校验（非 ENABLED →
// 10502）③bcrypt 校验（任何 error 一律译 10501——畸形 hash 也不 500，T-2-08）
// ④角色装配（会话 roles 载荷，permissions 不放，D-10）⑤发会话 ⑥出响应。
func (s *AuthService) Login(ctx context.Context, req *dto.LoginReq) (*dto.LoginResp, error) {
	u, err := s.userRepo.FindByUsername(ctx, derefStr(req.Username))
	if err != nil {
		return nil, err
	}
	if u == nil {
		// 用户不存在：仍执行一次真实 bcrypt 比较（恒时防枚举侧信道，T-2-01），
		// 之后与密码错误路径返回逐字节相同的 ErrBadCredentials。
		_ = bcrypt.CompareHashAndPassword(s.dummyHash, []byte(derefStr(req.Password)))
		return nil, errcode.ErrBadCredentials
	}
	if u.Status == nil || *u.Status != 1 {
		// 非 ENABLED（含 NULL fail-closed）：独立错误码 10502，消息照源"用户已被禁用"
		return nil, errcode.ErrUserDisabled
	}
	if err := bcrypt.CompareHashAndPassword([]byte(derefStr(u.Password)), []byte(derefStr(req.Password))); err != nil {
		// 畸形/过期/非 bcrypt 值（如存量 arm64 行的 10 字符垃圾值）一律登录失败
		// 而非 500（T-2-08）；消息与"用户不存在"完全一致（防枚举，D-02）。
		return nil, errcode.ErrBadCredentials
	}
	return s.completeLogin(ctx, u)
}

// Logout 登出：会话即刻失效（幂等，见 SessionManager.Logout）。
func (s *AuthService) Logout(ctx context.Context, token string) error {
	return s.sm.Logout(ctx, token)
}

// completeLogin 完整登录装配（Login 与 ApikeyLogin 共用私有路径）：一次 IN 查询取
// 角色码（两查询防 N+1 第一段，USER-05/AUTH-01 共用）→ SessionManager 发新 token →
// 装配响应（User 复用 UserResp，password/extToken 永不出网）。
func (s *AuthService) completeLogin(ctx context.Context, u *po.UserPO) (*dto.LoginResp, error) {
	roleMap, err := s.relationRepo.FindRolesByUserIDs(ctx, []int64{u.ID})
	if err != nil {
		return nil, err
	}
	roleCodes := make([]string, 0, len(roleMap[u.ID]))
	for _, r := range roleMap[u.ID] {
		roleCodes = append(roleCodes, r.RoleCode)
	}
	token, err := s.sm.Login(ctx, auth.SessionPayload{
		UserID:   u.ID,
		UserName: derefStr(u.Username),
		RealName: derefStr(u.RealName),
		Roles:    roleCodes,
	})
	if err != nil {
		return nil, err
	}
	return &dto.LoginResp{Token: token, User: dto.ToUserResp(u)}, nil
}

// derefStr nil 安全解引用（DB 列可空，PO 指针字段可能为 nil）。
func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
