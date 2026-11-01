package app

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/cool-wx/sys-iam-go/internal/authz"
	"github.com/cool-wx/sys-iam-go/internal/handler"
	"github.com/cool-wx/sys-iam-go/internal/health"
	"github.com/cool-wx/sys-iam-go/internal/middleware"
)

// handlers 路由聚合（Wave 2/3 扩域时按域增字段，newRouter 签名不再反复变化）。
type handlers struct {
	Auth           *handler.AuthHandler
	Users          *handler.UserHandler
	Menus          *handler.MenuHandler
	Depts          *handler.DeptHandler
	Roles          *handler.RoleHandler
	ApiKeys        *handler.ApiKeyHandler
	DictTypes      *handler.DictTypeHandler
	DictDatas      *handler.DictDataHandler
	Configs        *handler.ConfigHandler
	Shifts         *handler.ShiftHandler
	Files          *handler.FileHandler
	FileCategories *handler.FileCategoryHandler
	Ologs          *handler.OlogHandler
	Faults         *handler.FaultHandler
}

// newRouter 装配路由：四中间件全局链（RequestID→AccessLog→Recovery→BodyLimit）+
// 认证分组（D-15/AUTH-06 白名单收紧）：
//   - pub 组（免 token）：/healthz、/readyz、POST /api/v1/auth/login、
//     POST /api/v1/auth/apikey-login（白名单四路径齐备）；
//   - pri 组（authMw 全量校验）：POST /api/v1/auth/logout（会话撤销自身不设权限
//     门槛，挂载清单不含）+ 全部管理路由逐条 RequirePerm（Phase 3 PERM-06 全量
//     收口：34+2 路由逐路由 requirePerm，D-08/D-15；kickout 叠加 RequireRoles
//     纵深防御保留——WR-05/T-2-07 清偿）；
//   - 既有 users 四路由 + menus/depts 全套业务路由（02-02 D-11/D-13：复数资源
//     路由 + 树端点独立；gin 静态段 tree 与 :id 参数段共存，静态优先）；
//   - 操作日志（OLOG-01/03，Phase 4 04-03）：全部写路由逐条声明制挂 Oplog
//     中间件（D-05——module/operation 元数据随路由注册声明，等价源 @OperationLog
//     注解心智模型；叠加次序固定 RequirePerm 之后 handler 之前，鉴权失败不产生
//     日志）；GET /api/v1/logs 查询路由挂 PermLogList（读写分离——查询路由不记
//     操作日志）。
func newRouter(h *health.State, hs handlers, authMw gin.HandlerFunc, az authz.Authorizer, logger *slog.Logger, ologW *middleware.OpLogWriter) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	// multipart 内存阈值（FILE-02 工控机 RSS 平稳验收）：超 8MB 的部分 multipart
	// 自动落盘临时文件而非全量驻留内存（默认 32MB 过高）；全局 BodyLimit 对
	// multipart 跳过后，大文件上传的内存上界由此阈值 + upload 路由级 20MB 独立
	// 上限共同封顶。
	engine.MaxMultipartMemory = 8 << 20
	engine.Use(middleware.RequestID())
	engine.Use(middleware.AccessLog(logger))
	engine.Use(middleware.Recovery(logger))
	engine.Use(middleware.BodyLimit()) // 请求体 1MB 上限（WR-02/T-1-04 DoS 防护）

	pub := engine.Group("/")
	pub.GET("/healthz", h.Liveness)
	pub.GET("/readyz", h.Readiness)
	// auth 域写路由记操作日志（源存量实证 login 被记录——保真；login 未登录
	// identity 为 NULL，OLOG-02 集成断言分支）
	pub.POST("/api/v1/auth/login", middleware.Oplog(ologW, "AuthController", "login"), hs.Auth.Login)
	pub.POST("/api/v1/auth/apikey-login", middleware.Oplog(ologW, "AuthController", "apiKeyLogin"), hs.Auth.ApikeyLogin)

	pri := engine.Group("/", authMw)
	// logout 不挂 perm（挂载清单不含——会话撤销自身不设权限门槛）
	pri.POST("/api/v1/auth/logout", middleware.Oplog(ologW, "AuthController", "logout"), hs.Auth.Logout)
	// kickout 双层 gate（D-15 纵深防御不拆，照现行形态 RequireRoles 前置）：
	// RequireRoles(super_admin) 会话快照层 + requirePerm 授权层——仅 super_admin
	// 可调，不再"任何有效 token 可踢任意会话（含 admin）"；WR-05/T-2-07 清偿
	pri.POST("/api/v1/auth/kickout", middleware.RequireRoles("super_admin"), middleware.RequirePerm(az, authz.PermAuthKickout), middleware.Oplog(ologW, "AuthController", "kickout"), hs.Auth.Kickout)
	// 用户域九路由（PERM-06 全量收口，D-08 逐路由显式挂载，路由与权限同屏；
	// perm 串与 03-RESEARCH 挂载清单表逐行对照——reset-password 用 resetPwd 非 edit；
	// 六条写路由声明制挂 Oplog（D-05），三条读路由不记）
	pri.POST("/api/v1/users", middleware.RequirePerm(az, authz.PermUserAdd), middleware.Oplog(ologW, "UserController", "create"), hs.Users.Create)
	pri.PUT("/api/v1/users/:id", middleware.RequirePerm(az, authz.PermUserEdit), middleware.Oplog(ologW, "UserController", "update"), hs.Users.Update)
	pri.GET("/api/v1/users/:id", middleware.RequirePerm(az, authz.PermUserQuery), hs.Users.Get)
	pri.GET("/api/v1/users", middleware.RequirePerm(az, authz.PermUserList), hs.Users.List)
	pri.DELETE("/api/v1/users/:id", middleware.RequirePerm(az, authz.PermUserRemove), middleware.Oplog(ologW, "UserController", "delete"), hs.Users.Delete)
	// 用户装配子资源（USER-04/02/03/07，02-03）：roles 全量替换、status 停用踢会话、
	// reset-password 重置踢会话——gin 静态段（roles/status/reset-password）与 :id
	// 参数段共存，静态优先（与 roles/:id/menus 同构）
	pri.GET("/api/v1/users/:id/roles", middleware.RequirePerm(az, authz.PermUserQuery), hs.Users.GetRoles)
	pri.PUT("/api/v1/users/:id/roles", middleware.RequirePerm(az, authz.PermUserEdit), middleware.Oplog(ologW, "UserController", "assignRoles"), hs.Users.AssignRoles)
	pri.PUT("/api/v1/users/:id/status", middleware.RequirePerm(az, authz.PermUserEdit), middleware.Oplog(ologW, "UserController", "updateStatus"), hs.Users.UpdateStatus)
	pri.PUT("/api/v1/users/:id/reset-password", middleware.RequirePerm(az, authz.PermUserResetPwd), middleware.Oplog(ologW, "UserController", "resetPassword"), hs.Users.ResetPassword)

	// 菜单域六路由（MENU-01/02）：CRUD + 树端点（静态段 tree 与 :id 参数段共存；
	// tree 与列表均为 list 档，GET :id 为 query 档）；三条写路由记操作日志
	pri.POST("/api/v1/menus", middleware.RequirePerm(az, authz.PermMenuAdd), middleware.Oplog(ologW, "MenuController", "create"), hs.Menus.Create)
	pri.PUT("/api/v1/menus/:id", middleware.RequirePerm(az, authz.PermMenuEdit), middleware.Oplog(ologW, "MenuController", "update"), hs.Menus.Update)
	pri.GET("/api/v1/menus/tree", middleware.RequirePerm(az, authz.PermMenuList), hs.Menus.Tree)
	pri.GET("/api/v1/menus/:id", middleware.RequirePerm(az, authz.PermMenuQuery), hs.Menus.Get)
	pri.GET("/api/v1/menus", middleware.RequirePerm(az, authz.PermMenuList), hs.Menus.List)
	pri.DELETE("/api/v1/menus/:id", middleware.RequirePerm(az, authz.PermMenuRemove), middleware.Oplog(ologW, "MenuController", "delete"), hs.Menus.Delete)

	// 部门域六路由（DEPT-01/02）：与菜单域同构；三条写路由记操作日志
	pri.POST("/api/v1/depts", middleware.RequirePerm(az, authz.PermDeptAdd), middleware.Oplog(ologW, "DeptController", "create"), hs.Depts.Create)
	pri.PUT("/api/v1/depts/:id", middleware.RequirePerm(az, authz.PermDeptEdit), middleware.Oplog(ologW, "DeptController", "update"), hs.Depts.Update)
	pri.GET("/api/v1/depts/tree", middleware.RequirePerm(az, authz.PermDeptList), hs.Depts.Tree)
	pri.GET("/api/v1/depts/:id", middleware.RequirePerm(az, authz.PermDeptQuery), hs.Depts.Get)
	pri.GET("/api/v1/depts", middleware.RequirePerm(az, authz.PermDeptList), hs.Depts.List)
	pri.DELETE("/api/v1/depts/:id", middleware.RequirePerm(az, authz.PermDeptRemove), middleware.Oplog(ologW, "DeptController", "delete"), hs.Depts.Delete)

	// 角色域七路由（ROLE-01/02/03）：CRUD + 角色菜单分配子资源（全量替换语义；
	// :id 与 :id/menus GET 均为 query 档）；五条写路由记操作日志
	pri.POST("/api/v1/roles", middleware.RequirePerm(az, authz.PermRoleAdd), middleware.Oplog(ologW, "RoleController", "create"), hs.Roles.Create)
	pri.PUT("/api/v1/roles/:id", middleware.RequirePerm(az, authz.PermRoleEdit), middleware.Oplog(ologW, "RoleController", "update"), hs.Roles.Update)
	pri.GET("/api/v1/roles/:id", middleware.RequirePerm(az, authz.PermRoleQuery), hs.Roles.Get)
	pri.GET("/api/v1/roles", middleware.RequirePerm(az, authz.PermRoleList), hs.Roles.List)
	pri.DELETE("/api/v1/roles/:id", middleware.RequirePerm(az, authz.PermRoleRemove), middleware.Oplog(ologW, "RoleController", "delete"), hs.Roles.Delete)
	pri.GET("/api/v1/roles/:id/menus", middleware.RequirePerm(az, authz.PermRoleQuery), hs.Roles.GetMenus)
	pri.PUT("/api/v1/roles/:id/menus", middleware.RequirePerm(az, authz.PermRoleEdit), middleware.Oplog(ologW, "RoleController", "assignMenus"), hs.Roles.AssignMenus)
	// 数据权限范围部门子资源（PERM-03 档位 4 存储面，D-10 sys_role_dept）：独立
	// 子资源端点（与 roles/:id/menus 同形态，管理面先例是全删重建）；03-02 已挂，
	// 本路由计入全量收口总数
	pri.GET("/api/v1/roles/:id/scopes", middleware.RequirePerm(az, authz.PermRoleQuery), hs.Roles.GetScopes)
	pri.PUT("/api/v1/roles/:id/scopes", middleware.RequirePerm(az, authz.PermRoleEdit), middleware.Oplog(ologW, "RoleController", "assignScopes"), hs.Roles.AssignScopes)

	// API Key 域五路由（AKEY-01/02，02-03）：CRUD + status 子资源（源 editApiKey
	// 仅 status 单列语义）。至此 Phase 2 五域路由全量注册（D-11 六组齐备）；
	// 三条写路由记操作日志
	pri.POST("/api/v1/api-keys", middleware.RequirePerm(az, authz.PermApiKeyAdd), middleware.Oplog(ologW, "ApiKeyController", "create"), hs.ApiKeys.Create)
	pri.GET("/api/v1/api-keys/:id", middleware.RequirePerm(az, authz.PermApiKeyQuery), hs.ApiKeys.Get)
	pri.GET("/api/v1/api-keys", middleware.RequirePerm(az, authz.PermApiKeyList), hs.ApiKeys.List)
	pri.PUT("/api/v1/api-keys/:id/status", middleware.RequirePerm(az, authz.PermApiKeyEdit), middleware.Oplog(ologW, "ApiKeyController", "updateStatus"), hs.ApiKeys.UpdateStatus)
	pri.DELETE("/api/v1/api-keys/:id", middleware.RequirePerm(az, authz.PermApiKeyRemove), middleware.Oplog(ologW, "ApiKeyController", "delete"), hs.ApiKeys.Delete)

	// 字典类型域五路由（DICT-01，Phase 4 04-01）：CRUD 全挂 RequirePerm（D-04
	// Phase 3 收口纪律延续——无裸路由）；三条写路由记操作日志
	pri.POST("/api/v1/dict-types", middleware.RequirePerm(az, authz.PermDictTypeAdd), middleware.Oplog(ologW, "DictTypeController", "create"), hs.DictTypes.Create)
	pri.PUT("/api/v1/dict-types/:id", middleware.RequirePerm(az, authz.PermDictTypeEdit), middleware.Oplog(ologW, "DictTypeController", "update"), hs.DictTypes.Update)
	pri.GET("/api/v1/dict-types/:id", middleware.RequirePerm(az, authz.PermDictTypeQuery), hs.DictTypes.Get)
	pri.GET("/api/v1/dict-types", middleware.RequirePerm(az, authz.PermDictTypeList), hs.DictTypes.List)
	pri.DELETE("/api/v1/dict-types/:id", middleware.RequirePerm(az, authz.PermDictTypeRemove), middleware.Oplog(ologW, "DictTypeController", "delete"), hs.DictTypes.Delete)

	// 字典数据域五路由（DICT-02/03）：与字典类型同构；共用 PermDictType* 五串
	// （源 dictData 与 dictType 共用 dict:* 权限，不新增权限档）；三条写路由记操作日志
	pri.POST("/api/v1/dict-datas", middleware.RequirePerm(az, authz.PermDictTypeAdd), middleware.Oplog(ologW, "DictDataController", "create"), hs.DictDatas.Create)
	pri.PUT("/api/v1/dict-datas/:id", middleware.RequirePerm(az, authz.PermDictTypeEdit), middleware.Oplog(ologW, "DictDataController", "update"), hs.DictDatas.Update)
	pri.GET("/api/v1/dict-datas/:id", middleware.RequirePerm(az, authz.PermDictTypeQuery), hs.DictDatas.Get)
	pri.GET("/api/v1/dict-datas", middleware.RequirePerm(az, authz.PermDictTypeList), hs.DictDatas.List)
	pri.DELETE("/api/v1/dict-datas/:id", middleware.RequirePerm(az, authz.PermDictTypeRemove), middleware.Oplog(ologW, "DictDataController", "delete"), hs.DictDatas.Delete)

	// 系统配置域六路由（CONF-01）：CRUD + 按 key 读取端点（静态段 key 与 :id
	// 参数段共存，静态优先——与 menus/tree 同构先例；key/:key 为 query 档）；
	// 三条写路由记操作日志
	pri.POST("/api/v1/configs", middleware.RequirePerm(az, authz.PermConfigAdd), middleware.Oplog(ologW, "SysConfigController", "create"), hs.Configs.Create)
	pri.PUT("/api/v1/configs/:id", middleware.RequirePerm(az, authz.PermConfigEdit), middleware.Oplog(ologW, "SysConfigController", "update"), hs.Configs.Update)
	pri.GET("/api/v1/configs/key/:key", middleware.RequirePerm(az, authz.PermConfigQuery), hs.Configs.GetByKey)
	pri.GET("/api/v1/configs/:id", middleware.RequirePerm(az, authz.PermConfigQuery), hs.Configs.Get)
	pri.GET("/api/v1/configs", middleware.RequirePerm(az, authz.PermConfigList), hs.Configs.List)
	pri.DELETE("/api/v1/configs/:id", middleware.RequirePerm(az, authz.PermConfigRemove), middleware.Oplog(ologW, "SysConfigController", "delete"), hs.Configs.Delete)

	// 排班域五路由（SHFT-01/02）：CRUD；list 全量无分页（源行为直译）。
	// shift 域 perm 串为发明串（sys_menu 无对应项）——v1 仅 super_admin 可用
	// （matcher 豁免，Phase 3 auth/apiKey 先例，D-16）；三条写路由记操作日志
	pri.POST("/api/v1/shifts", middleware.RequirePerm(az, authz.PermShiftAdd), middleware.Oplog(ologW, "ShiftController", "create"), hs.Shifts.Create)
	pri.PUT("/api/v1/shifts/:id", middleware.RequirePerm(az, authz.PermShiftEdit), middleware.Oplog(ologW, "ShiftController", "update"), hs.Shifts.Update)
	pri.GET("/api/v1/shifts/:id", middleware.RequirePerm(az, authz.PermShiftQuery), hs.Shifts.Get)
	pri.GET("/api/v1/shifts", middleware.RequirePerm(az, authz.PermShiftList), hs.Shifts.List)
	pri.DELETE("/api/v1/shifts/:id", middleware.RequirePerm(az, authz.PermShiftRemove), middleware.Oplog(ologW, "ShiftController", "delete"), hs.Shifts.Delete)

	// 文件域四路由（FILE-01/02/03，Phase 4 04-02）：upload 叠加 20MB 路由级独立
	// 上限（全局 1MB 对 multipart 跳过后该类请求的唯一封顶，T-4-06；
	// download 挂 PermFileDownload——菜单存在此串非 super_admin 专属，D-16 核对结论）。
	// file 域 perm 串 sys_menu 齐备（7 项），实际可经角色授权；upload/delete 记
	// 操作日志（upload 为 multipart——中间件跳过缓存 params 空串，仅元数据落库）
	pri.GET("/api/v1/files", middleware.RequirePerm(az, authz.PermFileList), hs.Files.List)
	pri.POST("/api/v1/files/upload", middleware.BodyLimitWithMB(20), middleware.RequirePerm(az, authz.PermFileUpload), middleware.Oplog(ologW, "FileController", "upload"), hs.Files.Upload)
	pri.GET("/api/v1/files/download/:id", middleware.RequirePerm(az, authz.PermFileDownload), hs.Files.Download)
	pri.DELETE("/api/v1/files/:id", middleware.RequirePerm(az, authz.PermFileRemove), middleware.Oplog(ologW, "FileController", "delete"), hs.Files.Delete)

	// 文件分类域五路由（FILE-04）：CRUD + 树端点（静态段 tree 与 :id 参数段共存；
	// 树即列表档挂 PermFileList，GET :id 为 query 档——fileCategory 与 file 域
	// 源共用 file:* 权限，不新增权限档）；三条写路由记操作日志
	pri.POST("/api/v1/file-categories", middleware.RequirePerm(az, authz.PermFileAdd), middleware.Oplog(ologW, "FileCategoryController", "create"), hs.FileCategories.Create)
	pri.PUT("/api/v1/file-categories/:id", middleware.RequirePerm(az, authz.PermFileEdit), middleware.Oplog(ologW, "FileCategoryController", "update"), hs.FileCategories.Update)
	pri.GET("/api/v1/file-categories/tree", middleware.RequirePerm(az, authz.PermFileList), hs.FileCategories.Tree)
	pri.GET("/api/v1/file-categories/:id", middleware.RequirePerm(az, authz.PermFileQuery), hs.FileCategories.Get)
	pri.DELETE("/api/v1/file-categories/:id", middleware.RequirePerm(az, authz.PermFileRemove), middleware.Oplog(ologW, "FileCategoryController", "delete"), hs.FileCategories.Delete)

	// 操作日志查询路由（OLOG-03，Phase 4 04-03）：分页 1/10 特例 + 三 like 模糊
	// （handler 唯一 ParseWithDefaults 调用点）；读路由不记操作日志
	pri.GET("/api/v1/logs", middleware.RequirePerm(az, authz.PermLogList), hs.Ologs.List)

	// 故障域五路由（FAUL-01，Phase 4 04-04）：CRUD；list 分页 1/15 + code 索引列
	// eq 直传过滤。fault 域 perm 串为发明串（sys_menu 无对应项，D-16 研究核对
	// 结论）——v1 仅 super_admin 可用（matcher 豁免，shift 域先例）；三条写路由
	// 记操作日志（module=FaultInfoController 源类简名）
	pri.POST("/api/v1/faults", middleware.RequirePerm(az, authz.PermFaultAdd), middleware.Oplog(ologW, "FaultInfoController", "create"), hs.Faults.Create)
	pri.PUT("/api/v1/faults/:id", middleware.RequirePerm(az, authz.PermFaultEdit), middleware.Oplog(ologW, "FaultInfoController", "update"), hs.Faults.Update)
	pri.GET("/api/v1/faults/:id", middleware.RequirePerm(az, authz.PermFaultQuery), hs.Faults.Get)
	pri.GET("/api/v1/faults", middleware.RequirePerm(az, authz.PermFaultList), hs.Faults.List)
	pri.DELETE("/api/v1/faults/:id", middleware.RequirePerm(az, authz.PermFaultRemove), middleware.Oplog(ologW, "FaultInfoController", "delete"), hs.Faults.Delete)
	// Excel 导入（FAUL-02，静态段 import 与 :id 参数段共存静态优先——与 files/upload
	// 同构）：独立 20MB 体积上限（全局 1MB 对 multipart 跳过后该类请求的唯一封顶，
	// T-4-11）→ 权限 → 日志 → handler 次序；multipart 无 body 缓存，Oplog params 空串
	pri.POST("/api/v1/faults/import", middleware.BodyLimitWithMB(20), middleware.RequirePerm(az, authz.PermFaultImport), middleware.Oplog(ologW, "FaultInfoController", "import"), hs.Faults.Import)

	return engine
}
