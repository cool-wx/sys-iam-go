// Package service 角色域业务逻辑：CRUD + builtin 守卫（409/10602 零删除前置拦截）
// + 四连删事务（user_role→role_menu→role_dept→role，第三删档位 4 存储）
// + assignMenus/assignScopes 全量替换。
package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"github.com/cool-wx/sys-iam-go/internal/authz"
	"github.com/cool-wx/sys-iam-go/internal/event"
	"github.com/cool-wx/sys-iam-go/internal/model/dto"
	"github.com/cool-wx/sys-iam-go/internal/model/po"
	"github.com/cool-wx/sys-iam-go/pkg/errcode"
	"github.com/cool-wx/sys-iam-go/pkg/ids"
	"github.com/cool-wx/sys-iam-go/pkg/paging"
	"github.com/cool-wx/sys-iam-go/pkg/types"

	"github.com/cool-wx/sys-iam-go/internal/repository"
)

// RoleService 角色域服务。db 用于 service 层显式事务（四连删 / assignMenus 与
// assignScopes 全删重建——SkipDefaultTransaction 下唯一事务边界）；enf 用于业务
// 事务提交成功后的 Casbin 策略双轨同步（adapter 独立 Session 物理上不在
// 业务事务内，两独立提交）；deptRepo 供数据权限范围部门（档位 4）前置校验。
type RoleService struct {
	db           *gorm.DB
	roleRepo     *repository.RoleRepo
	menuRepo     *repository.MenuRepo
	relationRepo *repository.RelationRepo
	deptRepo     *repository.DeptRepo
	enf          *authz.Enforcer
	// pub 改名事件发布依赖（RoleNameChanged）：构造签名一次改齐
	//（照 perms.go 六域常量先例消除装配串行链），发布时序契约见 internal/event
	// 包注释——字段暂存不用（Go 允许未使用的 struct 字段）。
	pub event.Publisher
}

// NewRoleService 构造。
func NewRoleService(db *gorm.DB, roleRepo *repository.RoleRepo, menuRepo *repository.MenuRepo, relationRepo *repository.RelationRepo, deptRepo *repository.DeptRepo, enf *authz.Enforcer, pub event.Publisher) *RoleService {
	return &RoleService{db: db, roleRepo: roleRepo, menuRepo: menuRepo, relationRepo: relationRepo, deptRepo: deptRepo, enf: enf, pub: pub}
}

// Create 创建角色：服务端默认值 dataScope=1/status=ENABLED/builtin=false
// （源 Role.create 语义）；role_code 唯一冲突（uk_role_code，注意
// utf8mb4_general_ci 大小写不敏感）译为 400「角色编码已存在」。builtin=true
// 显式拒绝 400（builtin 只能由 seed/迁移流程设定——放行会让 role:add
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
		// TranslateError 开启后 MySQL 1062 → gorm.ErrDuplicatedKey
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return nil, errcode.New(errcode.CodeInvalidParam, "角色编码已存在")
		}
		return nil, err
	}
	return dto.ToRoleResp(role), nil
}

// Update 更新角色（部分更新）：白名单 role_name/data_scope/status/remark
// （nil=不写库）。builtin 刻意不在白名单（不可经 edit 翻转——否则 role:edit
// 持有者可先翻转 builtin=false 再 DELETE，builtin 删除守卫形同虚设，super_admin
// 的 g 行随四连删永久消失=全系统管理员豁免被摧毁；请求体 builtin 键被 DTO 白名单
// 忽略）。行不存在译为 404/10300「角色不存在: {id}」（源消息原文；code/HTTP 契约与
// 全库统一）。落库后回读返回最新契约；审计由 repo.Patch 显式并入。
//
// status 显式传入时的运行期策略同步（停用角色对授权即刻生效，家族
// 语义在角色轴的对应物——用户轴停用踢会话，角色轴停用清策略）：
//   - → DISABLED：Patch 落库后 ClearRole 双清该角色 p+g 行——持该角色的存量 token
//     下一请求即 403（g 行没了 matcher 豁免与 p 行均不命中），停用是授权即刻撤销；
//   - → ENABLED：Patch 落库后按业务表重建策略（SyncRolePerms p 行 + SyncRoleUsers
//     g 行，原料 FindPermsByRoleID/FindUserIDsByRoleID 与 boot 对账同口径）——恢复
//     即刻复原，无需等重启对账。
//
// 同步不做翻转检测而按目标值无条件执行：ClearRole/重建均幂等全删（全加），重复
// 提交（DISABLED→DISABLED、重复 ENABLED 恢复）无害；且 Patch 已提交后同步失败的
// 500 重试（status 值不变）仍会重跑同步——post-commit 失败可重试自愈（与 Delete
// 的 404 挡死不同，Patch 幂等）。业务表是唯一事实源：停用期间 sys_role_menu/
// sys_user_role 关联不动，恢复时全量重建即可。
//
// builtin × status 组合守卫（前置拦截零写调用，与 Delete 的 builtin 删除
// 守卫对称——builtin 角色不可删除，亦不可经 status 路径停用）：req.Status 为
// DISABLED 时先行查行校验 builtin，命中即 409/10604，Patch/ClearRole 一个都不触发。
// 无守卫时停用 builtin 角色会经 ClearRole 双清全部 super_admin 持有者的豁免 g 行
// （下一请求全 403），且 boot 对账 r.status=1 口径不还魂、role:edit 若仅 super_admin
// 持有则无人可恢复——重启不自愈的锁死（后果链）。仅拦 DISABLED：改名/档位/
// ENABLED 等更新不受影响（守卫零误伤）。
func (s *RoleService) Update(ctx context.Context, id int64, req *dto.RoleUpdateReq) (*dto.RoleResp, error) {
	// builtin 停用守卫：仅 status 显式 DISABLED 时触发，查行 fail-closed
	//（ErrNotFound 译 404 域消息，与 Patch 后置 404 语义一致）
	if req.Status != nil && *req.Status == types.UsageDisabled {
		existing, err := s.roleRepo.FindByID(ctx, id)
		if err != nil {
			if errors.Is(err, errcode.ErrNotFound) {
				return nil, errcode.New(errcode.CodeNotFound, fmt.Sprintf("角色不存在: %d", id))
			}
			return nil, err
		}
		if existing.Builtin != nil && *existing.Builtin {
			return nil, errcode.ErrBuiltinRoleStatus
		}
	}
	// 改名同步回读（源 RoleApplicationService.editRole 的 oldName
	// 取值前置同构）：仅当请求显式带角色名时回读旧行；行不存在提前 404 拦截，Patch
	// 与发布一个都不触发。与上方 builtin 停用守卫的 FindByID
	// **相互独立不合并**——守卫仅 status=DISABLED 触发，二者条件域不相交（本任务
	// 改名回读独立判断 req.RoleName != nil）。
	var old *po.RolePO
	if req.RoleName != nil {
		var err error
		old, err = s.roleRepo.FindByID(ctx, id)
		if err != nil {
			if errors.Is(err, errcode.ErrNotFound) {
				return nil, errcode.New(errcode.CodeNotFound, fmt.Sprintf("角色不存在: %d", id))
			}
			return nil, err
		}
	}
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
	// 改名事件发布（Patch 返回 nil 即已提交）：仅当本次请求带了角色名且回读
	// 成功即发布（契约修订：**不设**源 `roleName != null && !roleName.equals(oldName)`
	// 的新≠旧短路——主行已提交后重试同一请求时旧行已是新名，守卫恒假、事件永不
	// 重发——「重试幂等自愈」被证伪；显式带名即发布，幂等 UPDATE 使同名重发
	// 退化为 no-op。"仅改备注/档位"（不带角色名）仍零发布，非无条件发布）。RolePO
	// .RoleName 为非指针 string 列（DDL NOT NULL），无需空值兜底。Publish 失败上抛
	// 500（客户端重试同一请求必重发事件，幂等 UPDATE 自愈，沿 casbin 双轨
	// 纪律）。
	if old != nil && req.RoleName != nil {
		evt := event.NameChangedEvent{
			Type:    event.RoleNameChanged,
			ID:      id,
			OldName: old.RoleName,
			NewName: *req.RoleName,
		}
		if err := s.pub.Publish(ctx, evt); err != nil {
			return nil, err // → 500（同步 UPDATE 失败可观察，重试必重发自愈）
		}
	}
	// 运行期策略同步（仅 status 显式传入时触发；Patch 成功后执行，失败上抛 500
	// 可重试自愈——见上注）。roleCode 从本次 Patch 结果回读（Get 走同一查库路径，
	// 不二次查库直取）。
	if req.Status != nil {
		role, err := s.roleRepo.FindByID(ctx, id)
		if err != nil {
			return nil, err
		}
		if *req.Status == types.UsageDisabled {
			// 停用：双清 p+g 行（持该角色 token 下一请求即 403）
			if err := s.enf.ClearRole(role.RoleCode); err != nil {
				return nil, err
			}
		} else {
			// 启用（含从停用恢复）：按业务表全量重建 p+g 行
			perms, err := s.relationRepo.FindPermsByRoleID(ctx, id)
			if err != nil {
				return nil, err
			}
			if err := s.enf.SyncRolePerms(role.RoleCode, perms); err != nil {
				return nil, err
			}
			userIDs, err := s.relationRepo.FindUserIDsByRoleID(ctx, id)
			if err != nil {
				return nil, err
			}
			if err := s.enf.SyncRoleUsers(role.RoleCode, userIDs); err != nil {
				return nil, err
			}
		}
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
// （源特例）；envelope 照 paging.PageData 契约。
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
// 409/10602「系统内置角色不可删除」，四连删一个都不触发（源
// removeById 语义）；否则**预清模式**（修复裁决）：四连删事务提交前先
// enf.ClearRole 双清 casbin 该角色 p+g 行，再 db.Transaction 四连删
// user_role → role_menu → role_dept（档位 4 存储随角色连带清理）→ role
// （前三步源顺序，第四删插在第三步；任一失败整体回滚——多步关联写绝不
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
	// 预清：双清 casbin 该角色的 p 行 + g 行（级联撤权——builtin 守卫
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
		// 第四删：sys_role_dept 与 sys_role_menu 同构关联，角色删除连带清理
		if err := relationTx.DeleteRoleDepts(ctx, id); err != nil {
			return err
		}
		roleTx := repository.NewRoleRepo(tx)
		return roleTx.Delete(ctx, id)
	})
}

// AssignMenus 分配角色菜单（全量替换语义）：角色校验 → 停用写路径封口 →
// menuIds 前置全量校验（缺失集合译 400「菜单不存在: [逗号拼接]」，防脏关联）→
// db.Transaction{ DeleteRoleMenus + BatchCreateRoleMenus }——删旧插新同事务，杜绝
// "删后插前"窗口期角色权限全空的可见态；role_name/menu_name 冗余逐行
// 写入（源 RoleMenu.create 语义）。空数组=全回收（合法语义）。
func (s *RoleService) AssignMenus(ctx context.Context, roleID int64, menuIDs []ids.ID) error {
	role, err := s.roleRepo.FindByID(ctx, roleID)
	if err != nil {
		if errors.Is(err, errcode.ErrNotFound) {
			return errcode.New(errcode.CodeNotFound, fmt.Sprintf("角色不存在: %d", roleID))
		}
		return err
	}
	// 停用角色写路径封口（前置拦截零写调用）：停用即 ClearRole 双清授权
	// （停用语义），而运行期 matcher 只看 casbin p/g 行、无 status 概念——对停用
	// 角色再分配菜单会经 SyncRolePerms 把 p 行写回，停用状态被合法 API 复活。NULL
	// status 视作启用（照 UsageStatusFromInt「非 0 一律 ENABLED」读路径兜底；DDL
	// status DEFAULT '1'，NULL 实际不可达）。消息沿域 400 风格（「角色不存在」同款）。
	if role.Status != nil && *role.Status != 1 {
		return errcode.New(errcode.CodeInvalidParam, fmt.Sprintf("角色已停用，不可分配: %d", roleID))
	}
	// reqIDs map 去重（照 AssignRoles reqSeen 模板）：请求体重复 menuID 会让
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
	// 允许两菜单同 permission 串，去重是 AddPolicies 的硬前置）。
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
	// 双轨第二轨：事务提交成功后全量替换该角色 casbin p 策略（策略写
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

// AssignScopes 分配角色数据权限范围部门（档位 4 存储面，方案 A；
// 全量替换语义完全照 AssignMenus 同构——锁定 sys_role_menu 同构模式）：
// 角色校验（404「角色不存在: {id}」）→ deptIDs 前置全量校验（缺失集合译 400
// 「部门不存在: [逗号拼接]」，防脏关联）→ db.Transaction{ DeleteRoleDepts +
// BatchCreateRoleDepts }——删旧插新同事务；dept_name 以查库值
// 冗余写入（请求体不可伪造）。空数组=全回收（合法语义）。
// 本写路径不经 casbin 双轨（sys_role_dept 非 enforcer 消费数据，双轨仅覆盖
// 功能权限轴；档位变更的行过滤生效走 datascope 表驱动实时查询，无缓存）。
func (s *RoleService) AssignScopes(ctx context.Context, roleID int64, deptIDs []ids.ID) error {
	role, err := s.roleRepo.FindByID(ctx, roleID)
	if err != nil {
		if errors.Is(err, errcode.ErrNotFound) {
			return errcode.New(errcode.CodeNotFound, fmt.Sprintf("角色不存在: %d", roleID))
		}
		return err
	}
	// reqIDs map 去重（照 AssignRoles reqSeen 模板，同 AssignMenus 纪律）：
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
