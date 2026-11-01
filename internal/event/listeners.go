package event

import (
	"context"

	"gorm.io/gorm"

	"github.com/cool-wx/sys-iam-go/internal/model/po"
)

// SyncHandlers 名称同步 handler 集：每个 handler 在独立事务内执行确定性 UPDATE
// （D-02——handler 独立事务，物理上与发布者已提交的主事务先后分离，源 REQUIRES_NEW
// 行锁死锁面不存在）。持 *gorm.DB 直写不扩 repository：冗余同步非实体仓储语义，
// 列名为代码内字符串常量、值全占位符（T-1-03/T-1-08 纪律延伸，禁 fmt.Sprintf 拼 SQL）。
//
// 全部 UPDATE 恒为三段式 `SET 冗余列 = newName WHERE 关联列 = 事件ID AND 冗余列 <> newName`：
// 同一事件重放安全（已同步行不中、陈旧行被拉齐）；**恒不追加『空值列亦命中』放行
// 分支**（SQL `<>` 三值逻辑下 NULL 冗余行恒不中是源语义锁定——NULL 代表"从未冗余
// 过"，放行空值会让从未冗余过的行突然带值；SYNC-02 prohibition 明文禁该形态，
// 验收 grep 断言零命中）。
//
// 35 条 UPDATE 逐条落齐（照 RESEARCH 重点 1 源监听器逐表映射，不做 helper 收敛——
// 逐条显式形态即审计面，与源监听器逐行一一对应）。
type SyncHandlers struct {
	db *gorm.DB
}

// NewSyncHandlers 构造。
func NewSyncHandlers(db *gorm.DB) *SyncHandlers {
	return &SyncHandlers{db: db}
}

// RegisterAll 向 dispatcher 注册全部 6 个 handler（app.New 装配期一次调用）。
func RegisterAll(d *Dispatcher, db *gorm.DB) {
	h := NewSyncHandlers(db)
	d.Register(DeptNameChanged, h.OnDeptNameChanged)
	d.Register(DictTypeNameChanged, h.OnDictTypeNameChanged)
	d.Register(MenuNameChanged, h.OnMenuNameChanged)
	d.Register(RoleNameChanged, h.OnRoleNameChanged)
	d.Register(UsernameChanged, h.OnUsernameChanged)
	d.Register(RealNameChanged, h.OnUserRealNameChanged)
}

// OnDeptNameChanged 部门改名同步（源 DeptNameSyncListener 逐条映射，2 条）：
// sys_user.dept_name（dept_id 锚）+ sys_dept.parent_name（parent_id 锚）。
func (h *SyncHandlers) OnDeptNameChanged(ctx context.Context, evt NameChangedEvent) error {
	return h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. sys_user.dept_name WHERE dept_id={ID}
		if err := tx.Model(&po.UserPO{}).Where("dept_id = ? AND dept_name <> ?", evt.ID, evt.NewName).Update("dept_name", evt.NewName).Error; err != nil {
			return err
		}
		// 2. sys_dept.parent_name WHERE parent_id={ID}
		return tx.Model(&po.DeptPO{}).Where("parent_id = ? AND parent_name <> ?", evt.ID, evt.NewName).Update("parent_name", evt.NewName).Error
	})
}

// OnDictTypeNameChanged 字典类型改名同步（源 DictTypeNameSyncListener，1 条）：
// sys_dict_data.dict_type_name（dict_type_id 锚）。
func (h *SyncHandlers) OnDictTypeNameChanged(ctx context.Context, evt NameChangedEvent) error {
	return h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.Model(&po.DictDataPO{}).Where("dict_type_id = ? AND dict_type_name <> ?", evt.ID, evt.NewName).Update("dict_type_name", evt.NewName).Error
	})
}

// OnMenuNameChanged 菜单改名同步（源 MenuNameSyncListener，2 条）：
// sys_role_menu.menu_name（menu_id 锚）+ sys_menu.parent_name（parent_id 锚）。
func (h *SyncHandlers) OnMenuNameChanged(ctx context.Context, evt NameChangedEvent) error {
	return h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. sys_role_menu.menu_name WHERE menu_id={ID}
		if err := tx.Model(&po.RoleMenuPO{}).Where("menu_id = ? AND menu_name <> ?", evt.ID, evt.NewName).Update("menu_name", evt.NewName).Error; err != nil {
			return err
		}
		// 2. sys_menu.parent_name WHERE parent_id={ID}
		return tx.Model(&po.MenuPO{}).Where("parent_id = ? AND parent_name <> ?", evt.ID, evt.NewName).Update("parent_name", evt.NewName).Error
	})
}

// OnRoleNameChanged 角色改名同步（源 RoleNameSyncListener 2 条 + Go 扩展 1 条）：
// sys_role_menu.role_name + sys_user_role.role_name（源原样），另加
// sys_role_dept.role_name——该表是 Go 增量扩展（源不存在，Phase 3 D-10
// AssignScopes 写入 role_name 冗余），不同步则改名后该表冗余列陈旧；**Go 扩展
// 非源行为**（Q2 裁决在案）。
func (h *SyncHandlers) OnRoleNameChanged(ctx context.Context, evt NameChangedEvent) error {
	return h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. sys_role_menu.role_name WHERE role_id={ID}
		if err := tx.Model(&po.RoleMenuPO{}).Where("role_id = ? AND role_name <> ?", evt.ID, evt.NewName).Update("role_name", evt.NewName).Error; err != nil {
			return err
		}
		// 2. sys_user_role.role_name WHERE role_id={ID}
		if err := tx.Model(&po.UserRolePO{}).Where("role_id = ? AND role_name <> ?", evt.ID, evt.NewName).Update("role_name", evt.NewName).Error; err != nil {
			return err
		}
		// 3. sys_role_dept.role_name WHERE role_id={ID}——Go 扩展非源行为：
		// sys_role_dept 为 D-10 档位 4 存储表（源无此表），role_name 冗余随角色
		// 改名同步，Go 内一致性优先于字面对齐（RESEARCH Q2）。
		return tx.Model(&po.RoleDeptPO{}).Where("role_id = ? AND role_name <> ?", evt.ID, evt.NewName).Update("role_name", evt.NewName).Error
	})
}

// OnUsernameChanged 登录名变更同步（源 RealNameSyncListener.onUsernameChanged，1 条）：
// sys_user_role.username（user_id 锚）。
//
// **本 handler 在源内即死代码语义：无任何发布点。** 源 UsernameChangedEvent 全树
// 无 publish 调用（username 为登录锚点，创建后不可改，源 editUser 签名不含
// username）；Go 侧 UserService.Update 白名单刻意不含 username（user.go 注释在
// 案）。handler 与事件类型照源补齐契约（Q1 裁决），不发明触发路径——防"6 事件"
// 契约缺一。
//
// 注意同列双语义防混：本 handler 写的是**登录名**；OnUserRealNameChanged 对同一
// sys_user_role.username 列写的是**展示名**（源命名错位，见彼处注释）。
func (h *SyncHandlers) OnUsernameChanged(ctx context.Context, evt NameChangedEvent) error {
	return h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.Model(&po.UserRolePO{}).Where("user_id = ? AND username <> ?", evt.ID, evt.NewName).Update("username", evt.NewName).Error
	})
}

// OnUserRealNameChanged 用户真名变更同步（源 RealNameSyncListener 主路径，
// 14 张表 26 条 UPDATE，同一 Transaction 闭包内——照源 REQUIRES_NEW 单事务形态）：
//   - sys_user / sys_dept / sys_role / sys_menu / sys_dict_type / sys_dict_data /
//     sys_config / sys_api_key / sys_shift / sys_fault_info / sys_file_category /
//     sys_file 共 12 表：create_by_name + update_by_name 各 1 条（审计操作人列，
//     create_by_id/update_by_id 锚）；
//   - sys_user_role.username 1 条（user_id 锚）——**列名 username 实存展示名
//     realName**（源命名错位原样移植，勿"修正"列语义）；与 OnUsernameChanged 同列
//     不同值两语义（彼处写登录名、此处写展示名），双向注释防混；
//   - sys_operation_log.real_name 1 条（user_id 锚）——独立业务列非审计列
//     （该表刻意不内嵌 BaseWithOperator，po/operation_log.go 实证）。
func (h *SyncHandlers) OnUserRealNameChanged(ctx context.Context, evt NameChangedEvent) error {
	return h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1/14 sys_user：审计操作人名双列
		if err := tx.Model(&po.UserPO{}).Where("create_by_id = ? AND create_by_name <> ?", evt.ID, evt.NewName).Update("create_by_name", evt.NewName).Error; err != nil {
			return err
		}
		if err := tx.Model(&po.UserPO{}).Where("update_by_id = ? AND update_by_name <> ?", evt.ID, evt.NewName).Update("update_by_name", evt.NewName).Error; err != nil {
			return err
		}
		// 2/14 sys_user_role.username：列名 username、写入值为展示名（源命名错位，
		// 与 OnUsernameChanged 写登录名互为两语义——双向注释防混，prohibition
		// 明文禁"顺手修正"）
		if err := tx.Model(&po.UserRolePO{}).Where("user_id = ? AND username <> ?", evt.ID, evt.NewName).Update("username", evt.NewName).Error; err != nil {
			return err
		}
		// 3/14 sys_dept：审计操作人名双列
		if err := tx.Model(&po.DeptPO{}).Where("create_by_id = ? AND create_by_name <> ?", evt.ID, evt.NewName).Update("create_by_name", evt.NewName).Error; err != nil {
			return err
		}
		if err := tx.Model(&po.DeptPO{}).Where("update_by_id = ? AND update_by_name <> ?", evt.ID, evt.NewName).Update("update_by_name", evt.NewName).Error; err != nil {
			return err
		}
		// 4/14 sys_role：审计操作人名双列
		if err := tx.Model(&po.RolePO{}).Where("create_by_id = ? AND create_by_name <> ?", evt.ID, evt.NewName).Update("create_by_name", evt.NewName).Error; err != nil {
			return err
		}
		if err := tx.Model(&po.RolePO{}).Where("update_by_id = ? AND update_by_name <> ?", evt.ID, evt.NewName).Update("update_by_name", evt.NewName).Error; err != nil {
			return err
		}
		// 5/14 sys_menu：审计操作人名双列
		if err := tx.Model(&po.MenuPO{}).Where("create_by_id = ? AND create_by_name <> ?", evt.ID, evt.NewName).Update("create_by_name", evt.NewName).Error; err != nil {
			return err
		}
		if err := tx.Model(&po.MenuPO{}).Where("update_by_id = ? AND update_by_name <> ?", evt.ID, evt.NewName).Update("update_by_name", evt.NewName).Error; err != nil {
			return err
		}
		// 6/14 sys_dict_type：审计操作人名双列
		if err := tx.Model(&po.DictTypePO{}).Where("create_by_id = ? AND create_by_name <> ?", evt.ID, evt.NewName).Update("create_by_name", evt.NewName).Error; err != nil {
			return err
		}
		if err := tx.Model(&po.DictTypePO{}).Where("update_by_id = ? AND update_by_name <> ?", evt.ID, evt.NewName).Update("update_by_name", evt.NewName).Error; err != nil {
			return err
		}
		// 7/14 sys_dict_data：审计操作人名双列
		if err := tx.Model(&po.DictDataPO{}).Where("create_by_id = ? AND create_by_name <> ?", evt.ID, evt.NewName).Update("create_by_name", evt.NewName).Error; err != nil {
			return err
		}
		if err := tx.Model(&po.DictDataPO{}).Where("update_by_id = ? AND update_by_name <> ?", evt.ID, evt.NewName).Update("update_by_name", evt.NewName).Error; err != nil {
			return err
		}
		// 8/14 sys_config：审计操作人名双列
		if err := tx.Model(&po.SysConfigPO{}).Where("create_by_id = ? AND create_by_name <> ?", evt.ID, evt.NewName).Update("create_by_name", evt.NewName).Error; err != nil {
			return err
		}
		if err := tx.Model(&po.SysConfigPO{}).Where("update_by_id = ? AND update_by_name <> ?", evt.ID, evt.NewName).Update("update_by_name", evt.NewName).Error; err != nil {
			return err
		}
		// 9/14 sys_api_key：审计操作人名双列
		if err := tx.Model(&po.ApiKeyPO{}).Where("create_by_id = ? AND create_by_name <> ?", evt.ID, evt.NewName).Update("create_by_name", evt.NewName).Error; err != nil {
			return err
		}
		if err := tx.Model(&po.ApiKeyPO{}).Where("update_by_id = ? AND update_by_name <> ?", evt.ID, evt.NewName).Update("update_by_name", evt.NewName).Error; err != nil {
			return err
		}
		// 10/14 sys_shift：审计操作人名双列
		if err := tx.Model(&po.ShiftPO{}).Where("create_by_id = ? AND create_by_name <> ?", evt.ID, evt.NewName).Update("create_by_name", evt.NewName).Error; err != nil {
			return err
		}
		if err := tx.Model(&po.ShiftPO{}).Where("update_by_id = ? AND update_by_name <> ?", evt.ID, evt.NewName).Update("update_by_name", evt.NewName).Error; err != nil {
			return err
		}
		// 11/14 sys_fault_info：审计操作人名双列
		if err := tx.Model(&po.FaultInfoPO{}).Where("create_by_id = ? AND create_by_name <> ?", evt.ID, evt.NewName).Update("create_by_name", evt.NewName).Error; err != nil {
			return err
		}
		if err := tx.Model(&po.FaultInfoPO{}).Where("update_by_id = ? AND update_by_name <> ?", evt.ID, evt.NewName).Update("update_by_name", evt.NewName).Error; err != nil {
			return err
		}
		// 12/14 sys_file_category：审计操作人名双列
		if err := tx.Model(&po.FileCategoryPO{}).Where("create_by_id = ? AND create_by_name <> ?", evt.ID, evt.NewName).Update("create_by_name", evt.NewName).Error; err != nil {
			return err
		}
		if err := tx.Model(&po.FileCategoryPO{}).Where("update_by_id = ? AND update_by_name <> ?", evt.ID, evt.NewName).Update("update_by_name", evt.NewName).Error; err != nil {
			return err
		}
		// 13/14 sys_file（实体名 FileInfo，po/file.go 表名实证）：审计操作人名双列
		if err := tx.Model(&po.FileInfoPO{}).Where("create_by_id = ? AND create_by_name <> ?", evt.ID, evt.NewName).Update("create_by_name", evt.NewName).Error; err != nil {
			return err
		}
		if err := tx.Model(&po.FileInfoPO{}).Where("update_by_id = ? AND update_by_name <> ?", evt.ID, evt.NewName).Update("update_by_name", evt.NewName).Error; err != nil {
			return err
		}
		// 14/14 sys_operation_log.real_name：独立业务列（非 BaseWithOperator 审计列）
		return tx.Model(&po.OperationLogPO{}).Where("user_id = ? AND real_name <> ?", evt.ID, evt.NewName).Update("real_name", evt.NewName).Error
	})
}
