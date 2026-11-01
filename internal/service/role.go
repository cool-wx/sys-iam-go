// Package service 角色域业务逻辑：CRUD + builtin 守卫（409/10602 零删除前置拦截）
// + 四连删事务（user_role→role_menu→role_dept→role，D-10 第三删档位 4 存储）
// + assignMenus/assignScopes 全量替换（ROLE-01/02/03 + PERM-03）。
package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"github.com/cool-wx/sys-iam-go/internal/authz"
	"github.com/cool-wx/sys-iam-go/internal/model/dto"
	"github.com/cool-wx/sys-iam-go/internal/model/po"
	"github.com/cool-wx/sys-iam-go/pkg/errcode"
	"github.com/cool-wx/sys-iam-go/pkg/ids"
	"github.com/cool-wx/sys-iam-go/pkg/paging"

	"github.com/cool-wx/sys-iam-go/internal/repository"
)

// RoleService 角色域服务。db 用于 service 层显式事务（四连删 / assignMenus 与
// assignScopes 全删重建——SkipDefaultTransaction 下唯一事务边界）；enf 用于业务
// 事务提交成功后的 Casbin 策略双轨同步（D-13——adapter 独立 Session 物理上不在
// 业务事务内，两独立提交）；deptRepo 供数据权限范围部门（D-10 档位 4）前置校验。
type RoleService struct {
	db           *gorm.DB
	roleRepo     *repository.RoleRepo
	menuRepo     *repository.MenuRepo
	relationRepo *repository.RelationRepo
	deptRepo     *repository.DeptRepo
	enf          *authz.Enforcer
}

// NewRoleService 构造。
func NewRoleService(db *gorm.DB, roleRepo *repository.RoleRepo, menuRepo *repository.MenuRepo, relationRepo *repository.RelationRepo, deptRepo *repository.DeptRepo, enf *authz.Enforcer) *RoleService {
	return &RoleService{db: db, roleRepo: roleRepo, menuRepo: menuRepo, relationRepo: relationRepo, deptRepo: deptRepo, enf: enf}
}

// Create 创建角色：服务端默认值 dataScope=1/status=ENABLED/builtin=false
// （源 Role.create 语义，ROLE-01）；role_code 唯一冲突（uk_role_code，注意
// utf8mb4_general_ci 大小写不敏感）译为 400「角色编码已存在」。builtin=true
// 显式拒绝 400（WR-01：builtin 只能由 seed/迁移流程设定——放行会让 role:add
// 制造守卫永不可删的"永生角色"，删除守卫不变量被绕过）。
func (s *RoleService) Create(ctx context.Context, req *dto.RoleCreateReq) (*dto.RoleResp, error) {
	if req.Builtin != nil && *req.Builtin {
		return nil, errcode.New(errcode.CodeInvalidParam, "builtin 为系统保留字段，不可创建内置角色")
	}
	role := &po.RolePO{
		RoleName: *req.RoleName,
		RoleCode: *req.RoleCode,
		Remark:   req.Remark,
	}
	if req.DataScope != nil {
		role.DataScope = req.DataScope
	} else {
		ds := int8(1)
		role.DataScope = &ds
	}
	status := int8(1)
	role.Status = &status
	b := false
	role.Builtin = &b
	if err := s.roleRepo.Create(ctx, role); err != nil {
		// TranslateError 开启后 MySQL 1062 → gorm.ErrDuplicatedKey（P4）
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return nil, errcode.New(errcode.CodeInvalidParam, "角色编码已存在")
		}
		return nil, err
	}
	return dto.ToRoleResp(role), nil
}

// Update 更新角色（部分更新，ENG-05）：白名单 role_name/data_scope/status/remark
// （nil=不写库）。builtin 刻意不在白名单（WR-01：不可经 edit 翻转——否则 role:edit
// 持有者可先翻转 builtin=false 再 DELETE，builtin 删除守卫形同虚设，super_admin
// 的 g 行随四连删永久消失=全系统管理员豁免被摧毁；请求体 builtin 键被 DTO 白名单
// 忽略）。行不存在译为 404/10300「角色不存在: {id}」（源消息原文；code/HTTP 契约与
// Phase 1 一致）。落库后回读返回最新契约；审计由 repo.Patch 显式并入（ENG-02）。
func (s *RoleService) Update(ctx context.Context, id int64, req *dto.RoleUpdateReq) (*dto.RoleResp, error) {
	cols := map[string]any{}
	if req.RoleName != nil {
		cols["role_name"] = *req.RoleName
	}
	if req.DataScope != nil {
		cols["data_scope"] = *req.DataScope
	}
	if req.Status != nil {
		cols["status"] = int8(*req.Status)
	}
	if req.Remark != nil {
		cols["remark"] = *req.Remark
	}
	if err := s.roleRepo.Patch(ctx, id, cols); err != nil {
		if errors.Is(err, errcode.ErrNotFound) {
			return nil, errcode.New(errcode.CodeNotFound, fmt.Sprintf("角色不存在: %d", id))
		}
		return nil, err
	}
	return s.Get(ctx, id)
}

// Get 角色详情：未命中译为 404/10300「角色不存在: {id}」（源消息原文）。
func (s *RoleService) Get(ctx context.Context, id int64) (*dto.RoleResp, error) {
	role, err := s.roleRepo.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, errcode.ErrNotFound) {
			return nil, errcode.New(errcode.CodeNotFound, fmt.Sprintf("角色不存在: %d", id))
		}
		return nil, err
	}
	return dto.ToRoleResp(role), nil
}

// List 角色分页条件查询：roleName 语义（trim + LIKE + id ASC）由 repo 承担
// （源特例，ROLE-03）；envelope 照 paging.PageData 契约。
func (s *RoleService) List(ctx context.Context, page, size int, roleName string) (*paging.PageData[dto.RoleResp], error) {
	roles, total, err := s.roleRepo.List(ctx, page, size, roleName)
	if err != nil {
		return nil, err
	}
	items := make([]dto.RoleResp, 0, len(roles))
	for i := range roles {
		items = append(items, *dto.ToRoleResp(&roles[i]))
	}
	return &paging.PageData[dto.RoleResp]{Items: items, Total: total, Page: page, Size: size}, nil
}

// Delete 删除角色：builtin 守卫**前置拦截（零删除调用）**——builtin=true 直接
// 409/10602「系统内置角色不可删除」，四连删一个都不触发（ROLE-02/D-14，源
// removeById 语义）；否则**预清模式**（CR-02 修复裁决）：四连删事务提交前先
// enf.ClearRole 双清 casbin 该角色 p+g 行，再 db.Transaction 四连删
// user_role → role_menu → role_dept（D-10 档位 4 存储随角色连带清理）→ role
// （前三步源顺序，第四删插在第三步；任一失败整体回滚，T-2-10——多步关联写绝不
// 拆为多次独立提交）。
//
// 预清为何不可后置（原 post-commit ClearRole 缺陷）：清理放在事务提交后时，失败即
// 500 且角色行已删——客户端重试 DELETE 命中 FindByID→404 永不可自愈，该 roleCode
// 的 p/g 行残留至下次重启 boot 对账（工控机单机重启周期长，提权残留以周计），且无
// 任何日志留痕。预清模式下清理失败时角色行仍在，重试必重新进入本函数（ClearRole
// 幂等全删），重试即收敛；DB 删除成功后无任何 post-commit 步骤。已知代价（方向
// 安全，可接受）：清理成功而 DB 删除失败时角色短暂"零策略"，重删或重启对账均可恢复。
func (s *RoleService) Delete(ctx context.Context, id int64) error {
	role, err := s.roleRepo.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, errcode.ErrNotFound) {
			return errcode.New(errcode.CodeNotFound, fmt.Sprintf("角色不存在: %d", id))
		}
		return err
	}
	if role.Builtin != nil && *role.Builtin {
		return errcode.ErrBuiltinRole
	}
	// 预清：双清 casbin 该角色的 p 行 + g 行（D-13/T-3-01 级联撤权——builtin 守卫
	// 已保证不会对 builtin 角色走到这里）
	if err := s.enf.ClearRole(role.RoleCode); err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		relationTx := repository.NewRelationRepo(tx)
		if err := relationTx.DeleteUserRolesByRoleID(ctx, id); err != nil {
			return err
		}
		if err := relationTx.DeleteRoleMenus(ctx, id); err != nil {
			return err
		}
		// 第四删（D-10）：sys_role_dept 与 sys_role_menu 同构关联，角色删除连带清理
		if err := relationTx.DeleteRoleDepts(ctx, id); err != nil {
			return err
		}
		roleTx := repository.NewRoleRepo(tx)
		return roleTx.Delete(ctx, id)
	})
}

// AssignMenus 分配角色菜单（全量替换语义）：角色校验 → menuIds 前置全量校验
// （缺失集合译 400「菜单不存在: [逗号拼接]」，防脏关联）→ db.Transaction{
// DeleteRoleMenus + BatchCreateRoleMenus }——删旧插新同事务，杜绝"删后插前"
// 窗口期角色权限全空的可见态（T-2-11）；role_name/menu_name 冗余逐行写入
// （源 RoleMenu.create 语义）。空数组=全回收（合法语义）。
func (s *RoleService) AssignMenus(ctx context.Context, roleID int64, menuIDs []ids.ID) error {
	role, err := s.roleRepo.FindByID(ctx, roleID)
	if err != nil {
		if errors.Is(err, errcode.ErrNotFound) {
			return errcode.New(errcode.CodeNotFound, fmt.Sprintf("角色不存在: %d", roleID))
		}
		return err
	}
	// reqIDs map 去重（WR-02，照 AssignRoles reqSeen 模板）：请求体重复 menuID 会让
	// BatchCreateRoleMenus 在 sys_role_menu 插入完全重复行（两表仅普通索引无唯一键），
	// 脏数据持续累积且 GET /roles/:id/menuIds 原样回显失真。
	reqIDs := make([]int64, 0, len(menuIDs))
	reqSeen := make(map[int64]struct{}, len(menuIDs))
	for _, mid := range menuIDs {
		id := int64(mid)
		if _, dup := reqSeen[id]; dup {
			continue
		}
		reqSeen[id] = struct{}{}
		reqIDs = append(reqIDs, id)
	}
	menuNameOf := make(map[int64]string, len(reqIDs))
	// 同一次 FindByIDs 结果顺带收集 casbin p 行原料（勿二次查库）：Permission
	// 非空串 + map 去重（MySQL 8.4 同批重复行 Error 1869 整批失败——sys_menu
	// 允许两菜单同 permission 串，去重是 AddPolicies 的硬前置，T-3-01/D-13）。
	perms := make([]string, 0, len(reqIDs))
	permSeen := make(map[string]struct{}, len(reqIDs))
	if len(reqIDs) > 0 {
		menus, err := s.menuRepo.FindByIDs(ctx, reqIDs)
		if err != nil {
			return err
		}
		for i := range menus {
			name := ""
			if menus[i].Name != nil {
				name = *menus[i].Name
			}
			menuNameOf[menus[i].ID] = name
			if p := menus[i].Permission; p != nil && *p != "" {
				if _, dup := permSeen[*p]; !dup {
					permSeen[*p] = struct{}{}
					perms = append(perms, *p)
				}
			}
		}
	}
	var missing []string
	for _, rid := range reqIDs {
		if _, ok := menuNameOf[rid]; !ok {
			missing = append(missing, strconv.FormatInt(rid, 10))
		}
	}
	if len(missing) > 0 {
		return errcode.New(errcode.CodeInvalidParam, "菜单不存在: ["+strings.Join(missing, ", ")+"]")
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		relationTx := repository.NewRelationRepo(tx)
		if err := relationTx.DeleteRoleMenus(ctx, roleID); err != nil {
			return err
		}
		rows := make([]po.RoleMenuPO, 0, len(reqIDs))
		for _, rid := range reqIDs {
			rows = append(rows, po.RoleMenuPO{
				RoleID:   roleID,
				RoleName: role.RoleName,
				MenuID:   rid,
				MenuName: menuNameOf[rid],
			})
		}
		return relationTx.BatchCreateRoleMenus(ctx, rows)
	})
	if err != nil {
		return err
	}
	// 双轨第二轨：事务提交成功后全量替换该角色 casbin p 策略（D-13——策略写
	// 不在业务事务内，两独立提交；失败上抛 500 由客户端重试自愈，重试=幂等
	// 全删全加即收敛）。空 menuIds 时 perms 为空=只删不加（全回收语义自然成立）。
	return s.enf.SyncRolePerms(role.RoleCode, perms)
}

// GetMenuIDs 查询角色已分配的 menuId 列表（源 getRoleMenuIds 等价：无角色存在性
// 校验——未分配/不存在角色均返回空列表，源语义保真）。
func (s *RoleService) GetMenuIDs(ctx context.Context, roleID int64) ([]ids.ID, error) {
	menuIDs, err := s.relationRepo.FindMenuIDsByRoleID(ctx, roleID)
	if err != nil {
		return nil, err
	}
	out := make([]ids.ID, 0, len(menuIDs))
	for _, v := range menuIDs {
		out = append(out, ids.ID(v))
	}
	return out, nil
}

// AssignScopes 分配角色数据权限范围部门（PERM-03 档位 4 存储面，D-10 方案 A；
// 全量替换语义完全照 AssignMenus 同构——D-10 锁定 sys_role_menu 同构模式）：
// 角色校验（404「角色不存在: {id}」）→ deptIDs 前置全量校验（缺失集合译 400
// 「部门不存在: [逗号拼接]」，防脏关联）→ db.Transaction{ DeleteRoleDepts +
// BatchCreateRoleDepts }——删旧插新同事务（T-2-11 同款）；dept_name 以查库值
// 冗余写入（请求体不可伪造，T-2-15）。空数组=全回收（合法语义）。
// 本写路径不经 casbin 双轨（sys_role_dept 非 enforcer 消费数据，D-14 双轨仅覆盖
// 功能权限轴；档位变更的行过滤生效走 datascope 表驱动实时查询，无缓存）。
func (s *RoleService) AssignScopes(ctx context.Context, roleID int64, deptIDs []ids.ID) error {
	role, err := s.roleRepo.FindByID(ctx, roleID)
	if err != nil {
		if errors.Is(err, errcode.ErrNotFound) {
			return errcode.New(errcode.CodeNotFound, fmt.Sprintf("角色不存在: %d", roleID))
		}
		return err
	}
	// reqIDs map 去重（WR-02，照 AssignRoles reqSeen 模板，同 AssignMenus 纪律）：
	// 重复 deptID 会在 sys_role_dept 插入完全重复行。
	reqIDs := make([]int64, 0, len(deptIDs))
	reqSeen := make(map[int64]struct{}, len(deptIDs))
	for _, did := range deptIDs {
		id := int64(did)
		if _, dup := reqSeen[id]; dup {
			continue
		}
		reqSeen[id] = struct{}{}
		reqIDs = append(reqIDs, id)
	}
	deptNameOf := make(map[int64]string, len(reqIDs))
	if len(reqIDs) > 0 {
		depts, err := s.deptRepo.FindByIDs(ctx, reqIDs)
		if err != nil {
			return err
		}
		for i := range depts {
			name := ""
			if depts[i].DeptName != nil {
				name = *depts[i].DeptName
			}
			deptNameOf[depts[i].ID] = name
		}
	}
	var missing []string
	for _, did := range reqIDs {
		if _, ok := deptNameOf[did]; !ok {
			missing = append(missing, strconv.FormatInt(did, 10))
		}
	}
	if len(missing) > 0 {
		return errcode.New(errcode.CodeInvalidParam, "部门不存在: ["+strings.Join(missing, ", ")+"]")
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		relationTx := repository.NewRelationRepo(tx)
		if err := relationTx.DeleteRoleDepts(ctx, roleID); err != nil {
			return err
		}
		rows := make([]po.RoleDeptPO, 0, len(reqIDs))
		for _, did := range reqIDs {
			rows = append(rows, po.RoleDeptPO{
				RoleID:   roleID,
				RoleName: role.RoleName,
				DeptID:   did,
				DeptName: deptNameOf[did],
			})
		}
		return relationTx.BatchCreateRoleDepts(ctx, rows)
	})
}

// GetScopeDeptIDs 查询角色已配置的 deptId 列表（GET /roles/:id/scopes 契约）：
// 照 GetMenuIDs（无角色存在性校验——未配置/不存在角色均返回空列表，同轴语义）。
func (s *RoleService) GetScopeDeptIDs(ctx context.Context, roleID int64) ([]ids.ID, error) {
	deptIDs, err := s.relationRepo.FindDeptIDsByRoleID(ctx, roleID)
	if err != nil {
		return nil, err
	}
	out := make([]ids.ID, 0, len(deptIDs))
	for _, v := range deptIDs {
		out = append(out, ids.ID(v))
	}
	return out, nil
}
