// Package service 角色域业务逻辑：CRUD + builtin 守卫（409/10602 零删除前置拦截）
// + 三连删事务（user_role→role_menu→role）+ assignMenus 全量替换（ROLE-01/02/03）。
package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"github.com/cool-wx/sys-iam-go/internal/model/dto"
	"github.com/cool-wx/sys-iam-go/internal/model/po"
	"github.com/cool-wx/sys-iam-go/pkg/errcode"
	"github.com/cool-wx/sys-iam-go/pkg/ids"
	"github.com/cool-wx/sys-iam-go/pkg/paging"

	"github.com/cool-wx/sys-iam-go/internal/repository"
)

// RoleService 角色域服务。db 用于 service 层显式事务（三连删 / assignMenus
// 全删重建——SkipDefaultTransaction 下唯一事务边界）。
type RoleService struct {
	db           *gorm.DB
	roleRepo     *repository.RoleRepo
	menuRepo     *repository.MenuRepo
	relationRepo *repository.RelationRepo
}

// NewRoleService 构造。
func NewRoleService(db *gorm.DB, roleRepo *repository.RoleRepo, menuRepo *repository.MenuRepo, relationRepo *repository.RelationRepo) *RoleService {
	return &RoleService{db: db, roleRepo: roleRepo, menuRepo: menuRepo, relationRepo: relationRepo}
}

// Create 创建角色：服务端默认值 dataScope=1/status=ENABLED/builtin=false
// （源 Role.create 语义，ROLE-01）；role_code 唯一冲突（uk_role_code，注意
// utf8mb4_general_ci 大小写不敏感）译为 400「角色编码已存在」。
func (s *RoleService) Create(ctx context.Context, req *dto.RoleCreateReq) (*dto.RoleResp, error) {
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
	if req.Builtin != nil {
		role.Builtin = req.Builtin
	} else {
		b := false
		role.Builtin = &b
	}
	if err := s.roleRepo.Create(ctx, role); err != nil {
		// TranslateError 开启后 MySQL 1062 → gorm.ErrDuplicatedKey（P4）
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return nil, errcode.New(errcode.CodeInvalidParam, "角色编码已存在")
		}
		return nil, err
	}
	return dto.ToRoleResp(role), nil
}

// Update 更新角色（部分更新，ENG-05）：白名单 role_name/data_scope/status/
// builtin/remark（nil=不写库）——builtin 仅显式传入才覆写（源 withEdit 语义）。
// 行不存在译为 404/10300「角色不存在: {id}」（源消息原文；code/HTTP 契约与
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
	if req.Builtin != nil {
		cols["builtin"] = *req.Builtin
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
// 409/10602「系统内置角色不可删除」，三连删一个都不触发（ROLE-02/D-14，源
// removeById 语义）；否则 db.Transaction 三连删 user_role → role_menu → role
// （源顺序；任一失败整体回滚，T-2-10——多步关联写绝不拆为多次独立提交）。
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
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		relationTx := repository.NewRelationRepo(tx)
		if err := relationTx.DeleteUserRolesByRoleID(ctx, id); err != nil {
			return err
		}
		if err := relationTx.DeleteRoleMenus(ctx, id); err != nil {
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
	reqIDs := make([]int64, 0, len(menuIDs))
	for _, mid := range menuIDs {
		reqIDs = append(reqIDs, int64(mid))
	}
	menuNameOf := make(map[int64]string, len(reqIDs))
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
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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
