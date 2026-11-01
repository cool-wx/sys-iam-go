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
	"github.com/cool-wx/sys-iam-go/internal/authz"
	"github.com/cool-wx/sys-iam-go/internal/datascope"
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
// 停用/重置密码/删除踢存量会话（D-09 安全增强，CONTEXT.md 决策非漂移）；enf 用于
// Casbin g 策略双轨同步（D-13：AssignRoles 事务提交后写、Delete 事务提交前预清——
// CR-01 裁决，post-commit 清理失败不可自愈）。
type UserService struct {
	db           *gorm.DB
	repo         *repository.UserRepo
	relationRepo *repository.RelationRepo
	roleRepo     *repository.RoleRepo
	deptRepo     *repository.DeptRepo
	sm           *auth.SessionManager
	enf          *authz.Enforcer
}

// NewUserService 构造。
func NewUserService(db *gorm.DB, repo *repository.UserRepo, relationRepo *repository.RelationRepo, roleRepo *repository.RoleRepo, deptRepo *repository.DeptRepo, sm *auth.SessionManager, enf *authz.Enforcer) *UserService {
	return &UserService{
		db:           db,
		repo:         repo,
		relationRepo: relationRepo,
		roleRepo:     roleRepo,
		deptRepo:     deptRepo,
		sm:           sm,
		enf:          enf,
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

// List 用户列表：dataScope 档位解析（resolveScope）→ repo 分页过滤查询（Filter
// 注入 Count/Find 同闭包，P15）→ 角色两查询装配（USER-05）→ paging.PageData
// envelope（排序与钳位由 repo/paging 承担）。role 条件过滤本阶段不做（源
// selectPage 无 role 条件）。
func (s *UserService) List(ctx context.Context, page, size int) (*paging.PageData[dto.UserResp], error) {
	filter, err := s.resolveScope(ctx)
	if err != nil {
		return nil, err
	}
	users, total, err := s.repo.List(ctx, page, size, filter)
	if err != nil {
		return nil, err
	}
	items, err := s.assembleRoles(ctx, users)
	if err != nil {
		return nil, err
	}
	return &paging.PageData[dto.UserResp]{Items: items, Total: total, Page: page, Size: size}, nil
}

// resolveScope 解析当前调用者的 dataScope Filter（PERM-03，D-09/D-12——service
// 传档位、repo 层 Apply 注入，不侵入既有查询代码）：
//
//  1. identity 缺失（理论不可达——pri 组已挂 Auth）→ 零值 Filter（Mode 零值经
//     Apply 落 "1 = 0" 空结果 fail-closed，与 T-2-04 纪律一致）；
//  2. 查询者行缺失（删除踢会话的竞态窗口）→ 同上 fail-closed 空结果；
//  3. relationRepo.FindRoleScopesByUserIDs 单用户 IN → datascope.Widest 聚合
//     （多角色取 max，D-11；无角色/全 NULL → ModeSelf 最窄档 fail-closed）；
//  4. 档 3 → deptRepo.List + CollectSubtreeIDs 子树展开；档 4 → 对 DataScope=4
//     的角色集 FindDeptIDsByRoleIDs 取部门集**并集**（多角色档 4 集合并，去重）；
//     档 2 → 查询者部门单元素；查询者无部门时 DeptIDs 留空按 Apply "1 = 0"
//     fail-closed。
//
// 角色档位与部门集每次 List 实时查表（无缓存）——档位/部门集变更下一请求即按新
// 档位过滤（PERM-02 即时语义同轴）。super_admin 不做档位特判——严格表驱动
// （03-02 规划裁决记录 1；存量 super_admin 行 data_scope=NULL 由现场经既有
// PUT /roles/:id 修正）。
func (s *UserService) resolveScope(ctx context.Context) (datascope.Filter, error) {
	identity, ok := auth.FromContext(ctx)
	if !ok {
		return datascope.Filter{}, nil
	}
	u, err := s.repo.FindByID(ctx, identity.UserID)
	if err != nil {
		if errors.Is(err, errcode.ErrNotFound) {
			return datascope.Filter{}, nil
		}
		return datascope.Filter{}, err
	}
	scopes, err := s.relationRepo.FindRoleScopesByUserIDs(ctx, []int64{identity.UserID})
	if err != nil {
		return datascope.Filter{}, err
	}
	roleScopes := scopes[identity.UserID]
	dsPtrs := make([]*int8, 0, len(roleScopes))
	for _, sc := range roleScopes {
		dsPtrs = append(dsPtrs, sc.DataScope)
	}
	mode := datascope.Widest(dsPtrs)
	filter := datascope.Filter{Mode: mode, UserID: identity.UserID}
	switch mode {
	case datascope.ModeSelf, datascope.ModeAll:
		// 档 1 只认 create_by_id 无需部门集；档 5 无过滤
		return filter, nil
	case datascope.ModeDept:
		if u.DeptID != nil {
			filter.DeptIDs = []int64{*u.DeptID}
		}
		return filter, nil
	case datascope.ModeDeptSubtree:
		if u.DeptID == nil {
			return filter, nil // 部门集空 → Apply "1 = 0" fail-closed
		}
		depts, err := s.deptRepo.List(ctx)
		if err != nil {
			return datascope.Filter{}, err
		}
		filter.DeptIDs = datascope.CollectSubtreeIDs(depts, *u.DeptID)
		return filter, nil
	case datascope.ModeCustomDepts:
		roleIDs := make([]int64, 0, len(roleScopes))
		for _, sc := range roleScopes {
			if sc.DataScope != nil && *sc.DataScope == int8(datascope.ModeCustomDepts) {
				roleIDs = append(roleIDs, sc.RoleID)
			}
		}
		if len(roleIDs) == 0 {
			return filter, nil // 无档 4 角色配置 → 部门集空 → fail-closed
		}
		deptMap, err := s.relationRepo.FindDeptIDsByRoleIDs(ctx, roleIDs)
		if err != nil {
			return datascope.Filter{}, err
		}
		seen := make(map[int64]struct{}, len(deptMap))
		for _, ids := range deptMap {
			for _, did := range ids {
				if _, dup := seen[did]; !dup {
					seen[did] = struct{}{}
					filter.DeptIDs = append(filter.DeptIDs, did)
				}
			}
		}
		return filter, nil
	default:
		// 未知档（data_scope 域外值）——Filter 原样传 repo，Apply default 落 "1 = 0"
		return filter, nil
	}
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

// assignerIsSuperAdmin 判定当前操作者会话 roles 快照是否含 super_admin（CR-03
// builtin 分配守卫判据）。与 enforcer 豁免判据（D-07 g 层）刻意区分：此处是
// 【管理操作准入】判定（谁能分配 builtin 角色），非运行期授权判定——角色分配端点
// 已被 RequirePerm(system:user:edit) gate，会话快照仅做提权收紧的第二判据（会话
// roles 由登录装配，super_admin 角色回收后新登录即失去分配资格，窗口有限）。
// identity 缺失（防御分支，正常装配不可达）返回 false fail-closed。
func assignerIsSuperAdmin(ctx context.Context) bool {
	identity, ok := auth.FromContext(ctx)
	if !ok {
		return false
	}
	for _, rc := range identity.Roles {
		if rc == "super_admin" {
			return true
		}
	}
	return false
}

// AssignRoles 分配用户角色（全量替换语义，USER-04/D-12）：用户校验（「用户不存在:
// {id}」）→ roleRepo.FindByIDs 前置全量校验（缺失集合译 400「角色不存在: [逗号拼接]」，
// 消息格式照源；校验前置优于源"事务内先删后校验"，与 02-02 assignMenus 同决策）
// → builtin 提权守卫（CR-03：请求含 builtin 角色时要求操作者持 super_admin，否则
// 403/10603 前置拦截零写调用）→ db.Transaction{ DeleteUserRolesByUserID +
// BatchCreateUserRoles }——删旧插新同事务，杜绝"删后插前"窗口期。空数组=全回收
// （合法语义）。
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
	// reqIDs map 去重（Error 1869 防御，D-13）：请求体重复 roleID 会让
	// BatchCreateUserRoles 插重复行、AddGroupingPolicies 批内重复 g 行整批失败。
	reqIDs := make([]int64, 0, len(roleIDs))
	reqSeen := make(map[int64]struct{}, len(roleIDs))
	for _, rid := range roleIDs {
		id := int64(rid)
		if _, dup := reqSeen[id]; dup {
			continue
		}
		reqSeen[id] = struct{}{}
		reqIDs = append(reqIDs, id)
	}
	roleNameOf := make(map[int64]string, len(reqIDs))
	// 同一次 FindByIDs 结果顺带收集 casbin g 行原料 roleCode（勿二次查库）
	roleCodeOf := make(map[int64]string, len(reqIDs))
	// builtin 标志（CR-03 提权守卫判定原料，同次查询顺带收集）
	builtinOf := make(map[int64]*bool, len(reqIDs))
	if len(reqIDs) > 0 {
		roles, err := s.roleRepo.FindByIDs(ctx, reqIDs)
		if err != nil {
			return err
		}
		for i := range roles {
			roleNameOf[roles[i].ID] = roles[i].RoleName
			roleCodeOf[roles[i].ID] = roles[i].RoleCode
			builtinOf[roles[i].ID] = roles[i].Builtin
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
	// builtin 提权守卫（CR-03，前置拦截零写调用）：请求集合含 builtin 角色时要求
	// 操作者会话 roles 快照含 super_admin，否则 403/10603——堵住仅持 system:user:edit
	// 的操作者经合法角色分配端点把 super_admin 分配给自己/他人的提权链（g 行经
	// SyncUserRoles 即时双写，下一请求即生效；D-07 豁免判据在 enforcer g 层，拿到
	// super_admin g 行=拿到全权）。与 RoleService.Delete 的 builtin 守卫同哲学：
	// 前置拦截，业务表与 casbin 双轨零写入。identity 缺失（防御分支，正常装配必有）
	// 视为无 super_admin，fail-closed。普通角色分配完全不受影响。
	if !assignerIsSuperAdmin(ctx) {
		for _, rid := range reqIDs {
			if b := builtinOf[rid]; b != nil && *b {
				return errcode.ErrBuiltinRoleAssign
			}
		}
	}
	// 源命名错位：sys_user_role.username 列内容为展示名（realName 非空白优先，回退
	// username）——等价源 SaTokenConfig.java:120-137 审计操作人同款语义。
	display := derefStr(u.RealName)
	if strings.TrimSpace(display) == "" {
		display = derefStr(u.Username)
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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
	if err != nil {
		return err
	}
	// 双轨第二轨：事务提交成功后全量替换该用户 casbin g 策略（D-13 同步纪律同
	// AssignMenus——失败上抛 500 客户端重试自愈）。空 roleIds 时只删不加（全回收）。
	// g 行即时生效=PERM-02 验收核心：已登录用户不重登录，下一请求按新策略判定。
	roleCodes := make([]string, 0, len(reqIDs))
	for _, rid := range reqIDs {
		roleCodes = append(roleCodes, roleCodeOf[rid])
	}
	return s.enf.SyncUserRoles(u.ID, roleCodes)
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
// 前置拦截（源 removeById 语义，D-14）；否则**预清模式**（CR-01 修复裁决）：业务事务
// 提交前先撤销全部运行态授权——① sm.KickoutUser 踢存量会话 ② enf.ClearUser 清
// casbin g 行——再 db.Transaction{ DeleteUserRolesByUserID + user Delete }（先删关联
// 再删用户，源顺序，任一失败整体回滚）。
//
// 预清为何不可后置（原 post-commit 级联缺陷）：清理步骤放在事务提交后时，任一步失败
// 即 500 且用户行已删——客户端重试 DELETE 命中 FindByID→404，踢会话/清策略永远无法
// 补执行；认证中间件仅校验 Redis 会话存在性、不回查 DB（T-3-01 威胁模型），被删账号
// 的存量 token 在整个滚动 TTL 内继续完全可用（认证逃逸窗口）。预清模式下清理失败时
// 用户行仍在，重试必重新进入本函数，清理原语幂等（KickoutUser 惰性清理 + ClearUser
// 全删），重试即收敛；DB 删除成功后无任何 post-commit 步骤，重试 404 不再挡死清理。
//
// 已知代价（方向安全，可接受）：清理成功而 DB 删除失败时，用户行仍在但会话/g 行已被
// 撤——删除本就是全面收权操作，暂扣方向 fail-closed；重新登录可恢复会话，g 行经管理
// 员重新 AssignRoles 重建（同 UpdateStatus 踢会话的既有可逆语义）。
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
	// 预清第一步：踢存量会话（安全关键步骤，认证链只认 Redis 不回查 DB——先踢先失效）
	if err := s.sm.KickoutUser(ctx, id); err != nil {
		return err
	}
	// 预清第二步：清 casbin g 行（策略撤销，D-13 双轨的用户轴；T-3-01 删除即刻收权）
	if err := s.enf.ClearUser(id); err != nil {
		return err
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
