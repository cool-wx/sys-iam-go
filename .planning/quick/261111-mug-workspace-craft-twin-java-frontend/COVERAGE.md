# API Coverage — sys-iam-go REST API（前端移植消费面）

> Full coverage by default. Opt-outs are explicit, reasoned decisions.
> 被集成方：sys-iam-go Gin 后端（internal/app/router.go 全量路由面，基路径 /api/v1）。
> 集成方：从 ~/workspace/craft-twin-java/frontend 照搬裁剪后的管理台前端。

| capability | decision | reason |
|---|---|---|
| auth.login（POST /auth/login） | INTEGRATE | 登录页唯一入口 |
| auth.logout（POST /auth/logout） | INTEGRATE | Layout 退出登录 |
| auth.apikey-login（POST /auth/apikey-login） | OPT-OUT | 源前端无 API Key 登录界面 |
| auth.kickout（POST /auth/kickout） | OPT-OUT | 源前端无踢下线界面 |
| users.list / get / create / update / delete | INTEGRATE | 用户管理页主链路 |
| users.status（PUT /users/:id/status） | INTEGRATE | 用户启/停用开关 |
| users.reset-password（PUT /users/:id/reset-password） | INTEGRATE | 用户页重置密码按钮 |
| users.roles（GET/PUT /users/:id/roles） | INTEGRATE | 用户页角色多选保存/回显（Go 契约为子资源，替代源 create body roleIds） |
| menus.tree / list / get / create / update / delete | INTEGRATE | 菜单管理页；tree 兼作前端权限表派生数据源 |
| depts.tree / create / update / delete | INTEGRATE | 组织管理页 |
| depts.list（GET /depts 分页列表） | OPT-OUT | 页面只用 tree，分页列表无消费方 |
| roles.list / get / create / update / delete | INTEGRATE | 角色管理页主链路 |
| roles.menus（GET/PUT /roles/:id/menus） | INTEGRATE | 角色页菜单分配树 |
| roles.scopes（GET/PUT /roles/:id/scopes） | OPT-OUT | 源前端无数据权限范围编辑 UI |
| api-keys 全域（CRUD + status） | OPT-OUT | 源前端无 API Key 管理页（不发明源没有的页面） |
| dict-types list / create / update / delete | INTEGRATE | 字典管理页左表 |
| dict-types.get（GET /dict-types/:id） | OPT-OUT | 页面只走列表 |
| dict-datas list / create / update / delete | INTEGRATE | 字典管理页右表 |
| dict-datas.get（GET /dict-datas/:id） | OPT-OUT | 页面只走列表 |
| configs list / create / update / delete | INTEGRATE | 参数配置页 |
| configs.get（GET /configs/:id 与 /configs/key/:key） | OPT-OUT | 页面只走列表 |
| shifts 全域（CRUD） | OPT-OUT | 源前端无排班页 |
| files.list / upload / download / delete | INTEGRATE | 文件管理页 |
| file-categories.tree / create / update / delete | INTEGRATE | 文件管理页分类树 |
| file-categories.get（GET /file-categories/:id） | OPT-OUT | 页面只走 tree |
| logs.list（GET /logs） | INTEGRATE | 操作日志页 |
| faults 全域（CRUD + import） | OPT-OUT | 源 FaultPage 属 twin 故障知识库域（/twin/fault），非 sys-iam FaultInfo，不移植 |
| healthz / readyz | OPT-OUT | 基建探针，前端不消费 |

附注（源前端被删除域的后端能力缺口，非 sys-iam-go 路由面）：twin/mes/wcs/wms/ai/llm SSE/STOMP
等集成面随源域整块删除，不在本矩阵范围。
