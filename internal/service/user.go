// Package service 定义业务逻辑与事务边界（handler 只做协议转换，业务在此层）。
package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/cool-wx/sys-iam-go/internal/auth"
	"github.com/cool-wx/sys-iam-go/internal/model/dto"
	"github.com/cool-wx/sys-iam-go/internal/model/po"
	"github.com/cool-wx/sys-iam-go/internal/repository"
	"github.com/cool-wx/sys-iam-go/pkg/errcode"
	"github.com/cool-wx/sys-iam-go/pkg/ids"
	"github.com/cool-wx/sys-iam-go/pkg/paging"
	"github.com/cool-wx/sys-iam-go/pkg/types"
)

// UserService 用户域服务。db 用于 service 层显式事务（AssignRoles 全删重建 /
// Delete 先删关联再删用户——SkipDefaultTransaction 下唯一事务边界）；sm 用于
// 停用/重置密码踢存量会话（D-09 安全增强，CONTEXT.md 决策非漂移）。
type UserService struct {
	db           *gorm.DB
	repo         *repository.UserRepo
	relationRepo *repository.RelationRepo
	roleRepo     *repository.RoleRepo
	deptRepo     *repository.DeptRepo
	sm           *auth.SessionManager
}

// NewUserService 构造。
func NewUserService(db *gorm.DB, repo *repository.UserRepo, relationRepo *repository.RelationRepo, roleRepo *repository.RoleRepo, deptRepo *repository.DeptRepo, sm *auth.SessionManager) *UserService {
	return &UserService{
		db:           db,
		repo:         repo,
		relationRepo: relationRepo,
		roleRepo:     roleRepo,
		deptRepo:     deptRepo,
		sm:           sm,
	}
}

// validateDept 部门校验链（源 addUser/editUser 语义）：deptID==nil →
// 「部门不能为空」；查不到 → 「部门不存在: {id}」（消息原文保留）。返回查库得到的
// 部门行（deptName 以查库值冗余，不信请求体——防伪造 T-2-15）。
func (s *UserService) validateDept(ctx context.Context, deptID *ids.ID) (*po.DeptPO, error) {
	if deptID == nil {
		return nil, errcode.New(errcode.CodeInvalidParam, "部门不能为空")
	}
	id := int64(*deptID)
	dept, err := s.deptRepo.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, errcode.ErrNotFound) {
			return nil, errcode.New(errcode.CodeInvalidParam, fmt.Sprintf("部门不存在: %d", id))
		}
		return nil, err
	}
	return dept, nil
}

// Create 创建用户：DTO→PO（服务端默认值 Status=1/FirstLogin=true/Builtin=false，
// 源 User.create() 语义；password bcrypt 哈希化入库——与源 Hutool BCrypt 同为标准
// $2a$ 格式，存量哈希零迁移兼容），落库后 PO→DTO 返回。审计字段（双时间/四操作人/
// ID）由 PO 钩子服务端填充，不接受请求体任何同名客户端字段（ENG-02）。
// deptId 必填且必须存在（源 addUser 校验链）；deptName 以查库值覆写——请求体
// deptName 键被忽略（服务端冗余不可由请求伪造）。
func (s *UserService) Create(ctx context.Context, req *dto.UserCreateReq) (*dto.UserResp, error) {
	dept, err := s.validateDept(ctx, req.DeptID)
	if err != nil {
		return nil, err
	}
	u := &po.UserPO{
		Username: req.Username,
		RealName: req.RealName,
		Phone:    req.Phone,
		Email:    req.Email,
		DeptName: dept.DeptName, // 服务端查库冗余（源 addUser 语义）
	}
	// 密码哈希化（Phase 1 预留边界，Phase 2 收口）：明文绝不落库/出网；
	// 生成失败收敛为内部错误（不 500 裸抛，T-1-01 纪律）。
	if req.Password != nil {
		hash, err := bcrypt.GenerateFromPassword([]byte(*req.Password), bcrypt.DefaultCost)
		if err != nil {
			return nil, errcode.New(errcode.CodeInternal, "密码处理失败")
		}
		hashStr := string(hash)
		u.Password = &hashStr
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
//
// 白名单刻意不含 username/password 两列（源 editUser 不可改语义：用户名为登录锚点、
// 密码走 reset-password 端点）；deptId 显式传入时走与 Create 相同的校验链且
// dept_name 以查库值联动覆写（请求体 deptName 键在 deptId 存在时被查库值覆盖）。
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
	if req.DeptName != nil {
		cols["dept_name"] = *req.DeptName
	}
	if req.DeptID != nil {
		dept, err := s.validateDept(ctx, req.DeptID)
		if err != nil {
			return nil, err
		}
		cols["dept_id"] = int64(*req.DeptID)
		cols["dept_name"] = dept.DeptName // 查库值联动覆写（伪造的 deptName 键失效）
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

// assembleRoles 角色装配（USER-05 两查询防 N+1 唯一收口）：用户集查出后对
// relationRepo.FindRolesByUserIDs **一次性** IN 查询 → 内存组装——严禁调用方在
// 循环内逐用户查库（全列表恒定 2 条 SQL：Count + Find + IN 共 3 条为硬上界）。
// 空角色用户的 Roles 为 [] 非 null。
func (s *UserService) assembleRoles(ctx context.Context, users []po.UserPO) ([]dto.UserResp, error) {
	userIDs := make([]int64, 0, len(users))
	for i := range users {
		userIDs = append(userIDs, users[i].ID)
	}
	roleMap, err := s.relationRepo.FindRolesByUserIDs(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	items := make([]dto.UserResp, 0, len(users))
	for i := range users {
		r := dto.ToUserResp(&users[i])
		refs := roleMap[users[i].ID]
		roles := make([]dto.UserRoleRef, 0, len(refs))
		for _, ref := range refs {
			roles = append(roles, dto.UserRoleRef{RoleID: ids.ID(ref.RoleID), RoleName: ref.RoleName})
		}
		r.Roles = roles
		items = append(items, *r)
	}
	return items, nil
}

// List 用户列表：repo 分页查询 → 角色两查询装配（USER-05）→ paging.PageData
// envelope（排序与钳位由 repo/paging 承担，P15）。role 条件过滤本阶段不做
// （源 selectPage 无 role 条件）。
func (s *UserService) List(ctx context.Context, page, size int) (*paging.PageData[dto.UserResp], error) {
	users, total, err := s.repo.List(ctx, page, size)
	if err != nil {
		return nil, err
	}
	items, err := s.assembleRoles(ctx, users)
	if err != nil {
		return nil, err
	}
	return &paging.PageData[dto.UserResp]{Items: items, Total: total, Page: page, Size: size}, nil
}

// Get 用户详情：按主键查询后经同一装配路径（两查询）输出（未命中 404/10300，
// 译域「用户不存在: {id}」——与本服务其余单行方法一致）。
func (s *UserService) Get(ctx context.Context, id int64) (*dto.UserResp, error) {
	u, err := s.repo.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, errcode.ErrNotFound) {
			return nil, userNotFound(id)
		}
		return nil, err
	}
	items, err := s.assembleRoles(ctx, []po.UserPO{*u})
	if err != nil {
		return nil, err
	}
	return &items[0], nil
}

// notFoundErr 行不存在译域消息（repo ErrNotFound 哨兵 → 404/10300「用户不存在: {id}」，
// 源消息原文；code/HTTP 契约与 Phase 1 一致）。
func userNotFound(id int64) *errcode.Error {
	return errcode.New(errcode.CodeNotFound, fmt.Sprintf("用户不存在: %d", id))
}

// AssignRoles 分配用户角色（全量替换语义，USER-04/D-12）：用户校验（「用户不存在:
// {id}」）→ roleRepo.FindByIDs 前置全量校验（缺失集合译 400「角色不存在: [逗号拼接]」，
// 消息格式照源；校验前置优于源"事务内先删后校验"，与 02-02 assignMenus 同决策）
// → db.Transaction{ DeleteUserRolesByUserID + BatchCreateUserRoles }——删旧插新
// 同事务，杜绝"删后插前"窗口期。空数组=全回收（合法语义）。
//
// 行冗余列：role_name=角色名；username 列存 **realName 非空白优先回退 username**
// 的值——源 UserApplicationService.java:182-201 该列命名与内容不符（列名 username、
// 内容为展示名），原样移植并在此注释（勿"修正"列内容）。
func (s *UserService) AssignRoles(ctx context.Context, userID int64, roleIDs []ids.ID) error {
	u, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		if errors.Is(err, errcode.ErrNotFound) {
			return userNotFound(userID)
		}
		return err
	}
	reqIDs := make([]int64, 0, len(roleIDs))
	for _, rid := range roleIDs {
		reqIDs = append(reqIDs, int64(rid))
	}
	roleNameOf := make(map[int64]string, len(reqIDs))
	if len(reqIDs) > 0 {
		roles, err := s.roleRepo.FindByIDs(ctx, reqIDs)
		if err != nil {
			return err
		}
		for i := range roles {
			roleNameOf[roles[i].ID] = roles[i].RoleName
		}
	}
	var missing []string
	for _, rid := range reqIDs {
		if _, ok := roleNameOf[rid]; !ok {
			missing = append(missing, strconv.FormatInt(rid, 10))
		}
	}
	if len(missing) > 0 {
		return errcode.New(errcode.CodeInvalidParam, "角色不存在: ["+strings.Join(missing, ", ")+"]")
	}
	// 源命名错位：sys_user_role.username 列内容为展示名（realName 非空白优先，回退
	// username）——等价源 SaTokenConfig.java:120-137 审计操作人同款语义。
	display := derefStr(u.RealName)
	if strings.TrimSpace(display) == "" {
		display = derefStr(u.Username)
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		relationTx := repository.NewRelationRepo(tx)
		if err := relationTx.DeleteUserRolesByUserID(ctx, userID); err != nil {
			return err
		}
		rows := make([]po.UserRolePO, 0, len(reqIDs))
		for _, rid := range reqIDs {
			rows = append(rows, po.UserRolePO{
				UserID:   userID,
				Username: display,
				RoleID:   rid,
				RoleName: roleNameOf[rid],
			})
		}
		return relationTx.BatchCreateUserRoles(ctx, rows)
	})
}

// GetRoles 查询用户当前角色列表（USER-04）：用户校验 → FindRolesByUserIDs 单用户
// IN 查询 → []UserRoleRef（恒 [] 非 null）。
func (s *UserService) GetRoles(ctx context.Context, userID int64) ([]dto.UserRoleRef, error) {
	if _, err := s.repo.FindByID(ctx, userID); err != nil {
		if errors.Is(err, errcode.ErrNotFound) {
			return nil, userNotFound(userID)
		}
		return nil, err
	}
	roleMap, err := s.relationRepo.FindRolesByUserIDs(ctx, []int64{userID})
	if err != nil {
		return nil, err
	}
	refs := roleMap[userID]
	out := make([]dto.UserRoleRef, 0, len(refs))
	for _, ref := range refs {
		out = append(out, dto.UserRoleRef{RoleID: ids.ID(ref.RoleID), RoleName: ref.RoleName})
	}
	return out, nil
}

// UpdateStatus 更新用户状态（源 updateStatus 仅 status 单列语义）：Patch 单业务列 +
// **status==DISABLED 时 sm.KickoutUser 即刻踢存量会话**（D-09 CONTEXT.md 安全增强：
// 源不踢，停用用户持旧 token 继续操作是安全洞——T-2-13 缓解落点）；ENABLED 不动作。
func (s *UserService) UpdateStatus(ctx context.Context, id int64, status types.UsageStatus) error {
	if err := s.repo.Patch(ctx, id, map[string]any{"status": int8(status)}); err != nil {
		if errors.Is(err, errcode.ErrNotFound) {
			return userNotFound(id)
		}
		return err
	}
	if status == types.UsageDisabled {
		return s.sm.KickoutUser(ctx, id)
	}
	return nil
}

// ResetPassword 重置密码（USER-03，源 resetPassword 语义）：固定 123456 bcrypt 入库
// + first_login=true 两列 Patch；无旧密码校验（源行为）；成功后即刻踢存量会话
// （D-09 安全增强，T-2-14 缓解落点——密码可能已泄露期间旧会话不得存活）。
func (s *UserService) ResetPassword(ctx context.Context, id int64) error {
	if _, err := s.repo.FindByID(ctx, id); err != nil {
		if errors.Is(err, errcode.ErrNotFound) {
			return userNotFound(id)
		}
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("123456"), bcrypt.DefaultCost)
	if err != nil {
		return errcode.New(errcode.CodeInternal, "密码处理失败")
	}
	hashStr := string(hash)
	firstLogin := true
	if err := s.repo.Patch(ctx, id, map[string]any{"password": hashStr, "first_login": firstLogin}); err != nil {
		if errors.Is(err, errcode.ErrNotFound) {
			return userNotFound(id)
		}
		return err
	}
	return s.sm.KickoutUser(ctx, id)
}

// Delete 删除用户（USER-07）：builtin=true → errcode 单例 10601/409 **零删除调用**
// 前置拦截（源 removeById 语义，D-14）；否则 db.Transaction{ DeleteUserRolesByUserID
// + user Delete }——先删关联再删用户（源顺序），任一失败整体回滚。
func (s *UserService) Delete(ctx context.Context, id int64) error {
	u, err := s.repo.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, errcode.ErrNotFound) {
			return userNotFound(id)
		}
		return err
	}
	if u.Builtin != nil && *u.Builtin {
		return errcode.ErrBuiltinUser
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		relationTx := repository.NewRelationRepo(tx)
		if err := relationTx.DeleteUserRolesByUserID(ctx, id); err != nil {
			return err
		}
		userTx := repository.NewUserRepo(tx)
		return userTx.Delete(ctx, id)
	})
}
