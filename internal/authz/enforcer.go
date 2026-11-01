package authz

import (
	"strconv"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	gormadapter "github.com/casbin/gorm-adapter/v3"
	"gorm.io/gorm"
)

// Enforcer Casbin RBAC 授权引擎（进程内单例经 app.New 构造注入，D-02）：包一层
// SyncedEnforcer——gin 每请求一 goroutine 并发 Enforce + 管理端点并发写策略，
// 普通 Enforcer 有数据竞争，必用 Synced 版（D-13/03-RESEARCH 研究重点 2 实证）。
//
// 策略行设计（sub 采用 role_code，自洽解）：
//   - g 行 (userID 十进制串, role_code)、p 行 (role_code, perm 串)；
//   - role_code 不可改（RoleService.Update 白名单无 role_code）→ 策略行不因改名失效；
//   - builtin super_admin 角色不可删（守卫已有）→ 豁免判据恒定；
//   - Identity.Roles 快照与策略同轴（D-05 纯 RBAC 无 domain 维度）。
type Enforcer struct {
	inner *casbin.SyncedEnforcer
}

// Authorizer 授权判定最小接口（middleware.RequirePerm 依赖注入用，perm_test 可 fake）。
type Authorizer interface {
	Enforce(sub string, perm string) (bool, error)
}

// New 装配 enforcer（五步链，03-RESEARCH probe 全过实证；D-04 fail-fast——
// 授权基建不可用时服务绝不能以"全量 403"状态静默启动，对齐 app.New 的 Redis Ping 先例）：
//  1. gormadapter.TurnOffAutoMigrate(db)——必须在 NewAdapterByDB* 之前调用；
//     casbin_rule 表由 scripts/ddl/casbin_rule.sql 显式建表（schema 沿用，
//     AutoMigrate 会试图改既有 schema，CLAUDE.md 锁定）；
//  2. NewAdapterByDBUseTableName 复用既有 *gorm.DB 连接池（D-01）；
//  3. model.NewModelFromString 加载内嵌 RBAC model（D-03——注意
//     casbin.NewEnforcer 的 string 参数是文件路径不是内容，内嵌字符串只能走
//     NewModelFromString 构造路径）；
//  4. casbin.NewSyncedEnforcer 并发安全构造；
//  5. LoadPolicy 全量加载，失败原样上抛。
func New(db *gorm.DB) (*Enforcer, error) {
	gormadapter.TurnOffAutoMigrate(db)
	adapter, err := gormadapter.NewAdapterByDBUseTableName(db, "", "casbin_rule")
	if err != nil {
		return nil, err
	}
	m, err := model.NewModelFromString(rbacModel)
	if err != nil {
		return nil, err
	}
	inner, err := casbin.NewSyncedEnforcer(m, adapter)
	if err != nil {
		return nil, err
	}
	if err := inner.LoadPolicy(); err != nil {
		return nil, err
	}
	return &Enforcer{inner: inner}, nil
}

// Enforce 授权判定：sub 为 userID 十进制串（strconv.FormatInt），perm 为
// system:{domain}:{action} 串。进程内 matcher 求值，每请求零 SQL（策略全内存）。
func (e *Enforcer) Enforce(sub string, perm string) (bool, error) {
	return e.inner.Enforce(sub, perm)
}

// ReplaceAll 全量替换 p/g 策略（boot 对账入口，T-3-05）：服务启动时从
// sys_user_role + sys_role_menu 全量导入——casbin_rule 新表初始为空而存量关系
// 数据已有，无对账则全员 403 含管理员；同时修复运行期同步失败残留（重启自愈）。
// 幂等可重入：先枚举现有行全删再全加，len>0 守卫使空批路径不可达（Assumption
// A2）；adapter OnConflict DoNothing 使重复行安全（probe 实证）。
func (e *Enforcer) ReplaceAll(pRules, gRules [][]string) error {
	existingP, err := e.inner.GetPolicy()
	if err != nil {
		return err
	}
	if len(existingP) > 0 {
		if _, err := e.inner.RemovePolicies(existingP); err != nil {
			return err
		}
	}
	existingG, err := e.inner.GetGroupingPolicy()
	if err != nil {
		return err
	}
	if len(existingG) > 0 {
		if _, err := e.inner.RemoveGroupingPolicies(existingG); err != nil {
			return err
		}
	}
	if len(pRules) > 0 {
		if _, err := e.inner.AddPolicies(pRules); err != nil {
			return err
		}
	}
	if len(gRules) > 0 {
		if _, err := e.inner.AddGroupingPolicies(gRules); err != nil {
			return err
		}
	}
	return nil
}

// SyncRolePerms 全量替换角色的 p 策略（D-13：业务事务提交后调用，失败上抛 500
// 由客户端重试自愈——重试=幂等全删全加即收敛）。perms 必须业务层先去重
// （MySQL 8.4 同批重复行 Error 1869 整批失败，probe 实证）；空 perms=只删不加
// （全回收语义自然成立）。
func (e *Enforcer) SyncRolePerms(roleCode string, perms []string) error {
	if _, err := e.inner.RemoveFilteredPolicy(0, roleCode); err != nil {
		return err
	}
	if len(perms) > 0 {
		rules := make([][]string, 0, len(perms))
		for _, p := range perms {
			rules = append(rules, []string{roleCode, p})
		}
		if _, err := e.inner.AddPolicies(rules); err != nil {
			return err
		}
	}
	return nil
}

// SyncUserRoles 全量替换用户的 g 策略（D-13 同步纪律同 SyncRolePerms）；
// roleCodes 空=只删不加（角色全回收）。userID 经 strconv.FormatInt 转 g 行 sub 串。
func (e *Enforcer) SyncUserRoles(userID int64, roleCodes []string) error {
	uidStr := strconv.FormatInt(userID, 10)
	if _, err := e.inner.RemoveFilteredGroupingPolicy(0, uidStr); err != nil {
		return err
	}
	if len(roleCodes) > 0 {
		rules := make([][]string, 0, len(roleCodes))
		for _, rc := range roleCodes {
			rules = append(rules, []string{uidStr, rc})
		}
		if _, err := e.inner.AddGroupingPolicies(rules); err != nil {
			return err
		}
	}
	return nil
}

// ClearRole 双清角色的 p 行与 g 行（角色删除级联：RemoveFilteredGroupingPolicy
// 的 fieldIndex=1 是 role 位）。builtin 角色已被 service 守卫拦截，不会走到这里。
func (e *Enforcer) ClearRole(roleCode string) error {
	if _, err := e.inner.RemoveFilteredPolicy(0, roleCode); err != nil {
		return err
	}
	if _, err := e.inner.RemoveFilteredGroupingPolicy(1, roleCode); err != nil {
		return err
	}
	return nil
}

// ClearUser 清用户的全部 g 行（用户删除级联）。
func (e *Enforcer) ClearUser(userID int64) error {
	_, err := e.inner.RemoveFilteredGroupingPolicy(0, strconv.FormatInt(userID, 10))
	return err
}
