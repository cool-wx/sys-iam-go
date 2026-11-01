// Package authz 装配 Casbin RBAC 授权引擎（Phase 3 PERM-01..06）：进程内
// SyncedEnforcer 单例（D-02，单机单进程无 watcher）、纯 RBAC model 内嵌字符串
// （D-03/D-05）、策略持久化走 gorm-adapter 复用既有 GORM 连接（D-01）。
package authz

// rbacModel 纯 RBAC model（D-03 内嵌 const，单二进制交付禁部署目录文件依赖；
// D-05 无 domain 维度——单机单租户，加维度是无据复杂化）。
//
// 三大 matcher 坑（03-RESEARCH probe 实证，改动前必读）：
//  1. matcher token 名必须与 policy_definition 一致——p = role, perm 时只能写
//     p.role/p.perm，写 p.sub 报错 `No parameter 'p_sub' found.`；
//  2. super_admin 豁免必须是 matcher 的第一个 || 短路分支——写在后面时请求不
//     匹配任何 p 行则 matcher 从未被求值、豁免永不生效（probe 实测翻车后修正）；
//  3. g 层第二参支持字符串字面量 g(r.sub, "super_admin")——super_admin 豁免
//     判据完全走 enforcer g 层（D-07 唯一判据，不得用会话 roles 快照判定）：
//     撤销 super_admin 角色（g 行删除）后豁免下一请求立即失效。
const rbacModel = `
[request_definition]
r = sub, perm

[policy_definition]
p = role, perm

[role_definition]
g = _, _

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = g(r.sub, "super_admin") || (r.perm == p.perm && g(r.sub, p.role))
`

// perm 串常量全量清单（D-06：前缀实证为 system: 按本机 sys_menu 39 行真实数据，
// 非 sys:；格式 system:{domain}:{action}）。与 03-RESEARCH 挂载清单表（34 路由）
// 逐行对照；Plan 03-03 全量挂载时按该表取用。
//
// 已知边界（03-RESEARCH Open Question Q2，D-06 认可）：auth/apiKey 域六串为发明
// ——sys_menu 无对应 permission 串，assignMenus 无法给其他角色授权这两域，v1 仅
// super_admin 可管（matcher 豁免），Phase 4/5 前端联动时再补菜单项。
const (
	// auth 域（发明串，Q2 已知边界）
	PermAuthKickout = "system:auth:kickout"

	// user 域（sys_menu 实证：101 列表/1011 详情/1012 新增/1013 编辑/1014 删除/1015 重置密码）
	PermUserAdd      = "system:user:add"
	PermUserEdit     = "system:user:edit"
	PermUserQuery    = "system:user:query"
	PermUserList     = "system:user:list"
	PermUserRemove   = "system:user:remove"
	PermUserResetPwd = "system:user:resetPwd"

	// menu 域（103 列表/1031 详情/1032 新增/1033 编辑/1034 删除）
	PermMenuAdd    = "system:menu:add"
	PermMenuEdit   = "system:menu:edit"
	PermMenuQuery  = "system:menu:query"
	PermMenuList   = "system:menu:list"
	PermMenuRemove = "system:menu:remove"

	// dept 域（104 列表/1041 详情/1042 新增/1043 编辑/1044 删除）
	PermDeptAdd    = "system:dept:add"
	PermDeptEdit   = "system:dept:edit"
	PermDeptQuery  = "system:dept:query"
	PermDeptList   = "system:dept:list"
	PermDeptRemove = "system:dept:remove"

	// role 域（102 列表/1021 详情/1022 新增/1023 编辑/1024 删除）
	PermRoleAdd    = "system:role:add"
	PermRoleEdit   = "system:role:edit"
	PermRoleQuery  = "system:role:query"
	PermRoleList   = "system:role:list"
	PermRoleRemove = "system:role:remove"

	// apiKey 域（发明串，Q2 已知边界）
	PermApiKeyAdd    = "system:apiKey:add"
	PermApiKeyEdit   = "system:apiKey:edit"
	PermApiKeyQuery  = "system:apiKey:query"
	PermApiKeyList   = "system:apiKey:list"
	PermApiKeyRemove = "system:apiKey:remove"

	// dict 域（sys_menu 实证：105 列表/1051 详情/1052 新增/1053 编辑/1054 删除；
	// 04-RESEARCH D-04/D-16 perm 串核对结论：dict/config/log 三域菜单项齐全）。
	// dictData 共用同五串（源 dictData 与 dictType 共用 dict:* 权限，不新增权限档）
	PermDictTypeAdd    = "system:dict:add"
	PermDictTypeEdit   = "system:dict:edit"
	PermDictTypeQuery  = "system:dict:query"
	PermDictTypeList   = "system:dict:list"
	PermDictTypeRemove = "system:dict:remove"

	// config 域（sys_menu 实证，04-RESEARCH D-04/D-16 核对结论）
	PermConfigAdd    = "system:config:add"
	PermConfigEdit   = "system:config:edit"
	PermConfigQuery  = "system:config:query"
	PermConfigList   = "system:config:list"
	PermConfigRemove = "system:config:remove"

	// log 域（sys_menu 实证：list 单档——源 /log 仅 list/export 两端点且共用 list 档）
	PermLogList = "system:log:list"

	// file 域（sys_menu 实证 7 串齐全，id 107-1076：list/query/add/edit/remove/
	// upload/download；fileCategory 共用 file:* 同族串；upload/download 路由 04-02 挂）
	PermFileAdd      = "system:file:add"
	PermFileEdit     = "system:file:edit"
	PermFileQuery    = "system:file:query"
	PermFileList     = "system:file:list"
	PermFileRemove   = "system:file:remove"
	PermFileUpload   = "system:file:upload"
	PermFileDownload = "system:file:download"

	// shift 域（发明串，Q2 已知边界——sys_menu 无任何 shift:* 前缀菜单项
	// [04-RESEARCH D-04/D-16 核对结论：零行]，assignMenus 无法给其他角色授权
	// 此域，v1 仅 super_admin 可用（matcher 豁免），照 Phase 3 auth/apiKey 先例）
	PermShiftAdd    = "system:shift:add"
	PermShiftEdit   = "system:shift:edit"
	PermShiftQuery  = "system:shift:query"
	PermShiftList   = "system:shift:list"
	PermShiftRemove = "system:shift:remove"

	// fault 域（发明串，Q2 已知边界——sys_menu 无任何 fault:* 前缀菜单项
	// [04-RESEARCH D-04/D-16 核对结论：零行]，v1 仅 super_admin 可用，照
	// Phase 3 auth/apiKey 先例；import 为 Excel 导入独立档）
	PermFaultAdd    = "system:fault:add"
	PermFaultEdit   = "system:fault:edit"
	PermFaultQuery  = "system:fault:query"
	PermFaultList   = "system:fault:list"
	PermFaultRemove = "system:fault:remove"
	PermFaultImport = "system:fault:import"
)
