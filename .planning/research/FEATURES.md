# Feature Research

**Domain:** Java Spring Boot IAM 后端 → Go 全量移植（brownfield 行为基准研究）
**Researched:** 2026-10-31
**Confidence:** HIGH（全部结论直接阅读源码得出，非推测；文件路径见 Sources）

> 研究方法说明：本文件是源码行为清单，不是外部生态调研。所有 14 个 Controller、14 个 ApplicationService、
> 13 个实体、全部仓储实现、操作日志切面、6 个名称同步监听器、sys-kernel 公共设施（Sa-Token 配置 / 白名单 /
> UserContext / AutoFillHandler / Base 实体 / PageHelper / GlobalExceptionHandler / SystemPermissions）均逐一阅读。
> OAuth2 疑点已专项验证（见反特性章节）。

## Feature Landscape

### Table Stakes（移植验收必须 —— 接口+测试全过的基准）

以下 14 个域 + 4 项横切机制缺一即验收失败。按源码逐域列出端点与行为。

#### 1. Auth 认证（/auth）— 复杂度 HIGH

| 端点 | 行为 | 边缘案例 / 易丢失细节 |
|------|------|----------------------|
| POST /auth/login | username+password → 查用户 → 校验状态 ENABLED → BCrypt 校验 → 生成 token → 会话加载 | 失败统一报"用户名或密码错误"（不区分不存在/密码错）；禁用用户报"用户已被禁用"；**无账号锁定、无密码强度规则、无验证码** |
| POST /auth/apikeyLogin | apiKey → 仓储按 key+ENABLED+未过期查询 → 关联用户登录 | 若该用户已有会话且权限缓存存在则直接复用（跳过重查库）；关联用户不存在/禁用报错 |
| POST /auth/logout | 注销当前 token | 需登录 |
| GET /auth/userInfo | 返回 `{user, permissions}` | password/extToken 为 WRITE_ONLY 不出现在 JSON |
| GET /auth/routes | super_admin → 全部 ENABLED 菜单；否则按角色查（visible=true + ENABLED，sort 升序）→ 内存建树 | 树构建：parentId 为 null/0 为根；**父 ID 不存在时提升为根**（孤儿挂根） |
| POST /auth/refresh | 读当前 token 剩余 ttl 并续期，返回新 ttl | — |

**关键移植语义（必保）**：登录时把 `permissions`、`roles`、`username`、`realName` **快照写入会话（Redis）**；
之后所有权限校验读会话快照而**不是实时查库** —— 改权限/角色后需重新登录才生效。无刷新 token 双令牌机制。

#### 2. User 用户（/user）— 复杂度 HIGH

| 端点 | 行为 | 边缘案例 / 易丢失细节 |
|------|------|----------------------|
| GET /user/list | 分页 + username/realName/phone 模糊 + status/deptId 等值 + createTime 倒序 | 返回 **UserVo（剔除 password/extToken）** 并批量装配角色列表（user_role IN + role IN 两次查询，**禁 N+1**） |
| GET /user/get/{id} | 返回原始 User | 同 WRITE_ONLY 脱敏 |
| POST /user/listByIds | ID 批量查 | 空列表直接返回空 |
| POST /user/add | deptId **必填**（不存在报"部门不能为空"/"部门不存在"）→ BCrypt 加密 → status=ENABLED、firstLogin=true、builtin 默认 false → 批量分配角色 | 角色分配校验所有 roleId 存在（报"角色不存在: [ids]"），并冗余写入 roleName/username 到 user_role |
| PUT /user/edit | **不可改 username/password**；deptId 变更重解析 deptName；realName 变更发 `UserRealNameChangedEvent`；roleIds 非 null 时全删重建 | builtin 字段仅显式传入才覆写（省略键不清掉内置标志） |
| DELETE /user/delete/{id} | **builtin=true 硬拦截**（"系统内置用户不可删除"，零副作用）→ 先删 user_role 关联 → 删用户 | 物理删除 |
| PUT /user/resetPwd/{id} | 重置为固定值 **123456**（BCrypt）+ firstLogin=true | 无旧密码校验 |
| PUT /user/status | ENABLED/DISABLED 切换 | **禁用不踢既有会话**（源无此逻辑） |

#### 3. Role 角色（/role）— 复杂度 MEDIUM

| 端点 | 行为 | 边缘案例 / 易丢失细节 |
|------|------|----------------------|
| GET /role/list | roleName 模糊 + 分页 | — |
| GET /role/get/{id} | 单查 | — |
| POST /role/add | roleName+roleCode 必填；dataScope 默认 1；status=ENABLED | — |
| PUT /role/edit | 非 null 字段才更新；roleName 变更发 `RoleNameChangedEvent` | — |
| DELETE /role/delete/{id} | **builtin 硬拦截** → 三连删：user_role → role_menu → role，事务回滚 | — |
| POST /role/menus | 分配菜单：校验角色存在 → 全删重建 role_menu → 校验所有 menuId 存在 → 冗余写入 roleName/menuName | menuIds @NotEmpty |
| GET /role/menus/{id} | 已分配菜单 ID 列表 | — |

`dataScope` 字段仅存储（默认 1），**源系统没有任何数据权限过滤实现**（见反特性）。

#### 4. Menu 菜单（/menu）— 复杂度 MEDIUM

| 端点 | 行为 | 边缘案例 |
|------|------|---------|
| GET /menu/tree | 全量（sort 升序）→ 内存建树 | 孤儿提升为根；children 为非库字段 |
| GET /menu/get/{id} | 单查 | — |
| POST /menu/add | parentId 非 0 时校验父菜单存在 → 冗余写 parentName | 无重复校验 |
| PUT /menu/edit | name 变更发 `MenuNameChangedEvent` | — |
| DELETE /menu/delete/{id} | 直接删 | **无子菜单/角色引用检查**（源就这样，可留缺口也可改进，但测试基准按源行为） |

MenuType 枚举：DIR(0)/MENU(1)/BUTTON(2)，按 value 存库、JSON 序列化为 value（EnumByValue 约定）。

#### 5. Dept 部门（/dept）— 复杂度 MEDIUM

与 Menu 完全同构：tree / get / add（父校验+parentName）/ edit（`DeptNameChangedEvent`）/ delete（无引用检查）。
Dept 排序字段 sort，status 字段。

#### 6. DictType 字典类型（/dictType）— 复杂度 LOW

全量 list（不分页）/ get / add / edit（`DictTypeNameChangedEvent`）/ delete。
**删除不级联删除字典数据**（源行为）。实体在 sys-kernel（DictType）。

#### 7. DictData 字典数据（/dictData）— 复杂度 LOW

| 端点 | 行为 |
|------|------|
| GET /dictData/list | 可选 `?dictTypeId=` 过滤；无参则全量 |
| get/add/edit/delete | add/edit 时解析 dictTypeName 冗余（typeId 非空必须存在）；删除不校验引用 |

另有仓储方法 `listDataByDictType(String)` / `getDictDataByDictTypeAndValue`（按 type 编码+value 精确查）——
仅被 Dubbo RPC 使用，HTTP 层无暴露（RPC 不移植，见反特性）。

#### 8. Config 系统配置（/config）— 复杂度 LOW

全量 list / get/{id} / **GET /config/key/{key} 按键取值（返回 string 值）** / add / edit / delete。
无缓存、无变更事件、无 key 唯一性校验（靠 DB）。

#### 9. OperationLog 操作日志（/log）— 复杂度 HIGH（含切面）

**AOP 切面行为（Gin 中间件等价实现的目标语义）**：
- 拦截**所有** Controller 方法（`interfaces.controller..*`）
- 记录字段：userId + realName（从 Sa-Token 会话取，未登录留空）、ip（remoteAddr）、`method`（HTTP 方法 + URI）、
  `module`（Controller 类简名）、`operation`（方法名）、`params`（前 3 个非 Servlet/非 MultipartFile 参数的 JSON，`; ` 拼接）、
  status（成功 1 / 异常 0）、errorMsg（截断 500 字符）、duration(ms)
- **敏感字段掩码**：password/oldPassword/newPassword/secretKey/apiKey/token/accessToken/refreshToken → `"******"`（正则替换 JSON 字符串值）
- **保存失败静默吞掉**（不污染主流程响应）；异常时先抛原异常再 finally 存日志

**查询**：
| 端点 | 行为 | 易丢失细节 |
|------|------|-----------|
| GET /log/list | module/operation/realName 模糊 + 分页 createTime 倒序 | **LogQuery 自带默认 pageNum=1/pageSize=10**（与其他域 PageHelper 默认 1/15 不同） |
| GET /log/export | 同过滤条件全量 Excel 导出（application/vnd...spreadsheetml.sheet，文件名"操作日志"） | Apache Fesod 注解驱动（@ExcelProperty 中文表头） |

#### 10. File 文件（/file）— 复杂度 HIGH（对象存储抽象）

| 端点 | 行为 | 边缘案例 / 易丢失细节 |
|------|------|----------------------|
| GET /file/list | categoryId 等值过滤 + 分页（默认 1/15）createTime 倒序 | — |
| POST /file/upload | multipart；分类校验**前置**（拒绝时零副作用）→ **服务端自算 MD5**（不信任客户端）→ 同分类同 hash 拒绝（"文件已存在：xxx"）→ DB save → RustFS(S3) upload；文件路径 = `UUID_原始文件名`；fileType 取扩展名（无扩展名为 ""） | 判重查询含 **categoryId IS NULL 分支**（未分类上传也要判重——唯一索引对 NULL 不生效的补偿）；异常时事务回滚 DB，但 S3 已传对象成孤儿（源未处理） |
| GET /file/download/{id} | 全量字节读进内存 → Content-Disposition attachment（原始文件名） | 路径为空报 IllegalStateException |
| DELETE /file/delete/{id} | 先删 S3 对象（**失败仅 warn 日志，继续删 DB 记录**） | 宽容删除语义 |
| 启动时 | createBucket（@PostConstruct） | Go 版改为存储初始化钩子 |

Go 版要求（PROJECT.md）：存储抽象 `本地磁盘 / RustFS(S3)` 可配置切换 —— 上传/下载/删除三操作 + bucket 初始化即抽象面。

#### 11. FileCategory 文件分类（/fileCategory）— 复杂度 LOW

树 CRUD 同 Dept 模式（父校验+parentName 冗余）。**注意源缺陷**：categoryName 变更**没有**名称同步事件 ——
FileInfo.categoryName 与子分类 parentName 改名后变陈旧。移植保持 schema 原样即可，不必补此功能。

#### 12. Shift 班次（/shift）— 复杂度 LOW（项目特有：WCS/MES）

纯 CRUD 直通仓储：list（**全量无分页**）/ get / add / edit / delete。
字段：shiftCode、shiftName、startTime/endTime（LocalTime）、status。
**shiftCode 无唯一性校验**（getByShiftCode 方法存在但仅 RPC 用）。7 个服务方法直通，无业务规则。

#### 13. FaultInfo 故障信息（/faultInfo）— 复杂度 MEDIUM（项目特有：WCS/MES）

| 端点 | 行为 | 易丢失细节 |
|------|------|-----------|
| GET /faultInfo/list | 分页（默认 1/15） | 无任何过滤条件 |
| get/add/edit/delete | 直通 CRUD | — |
| POST /faultInfo/import | Excel 导入，按 **name + bitIndex + location 三键** upsert：匹配→update（updateCount++），否则 insert（insertCount++）；name 为 null 的行**静默跳过**；返回 `ImportResult{insertCount, updateCount}`；@Transactional 整体 | 无行级错误报告（源就这样） |
| GET /faultInfo/export | 全量 Excel 导出，文件名"故障库" | FAULT_EXPORT 独立权限 |

实体在 sys-kernel（FaultInfo），含 faultType 枚举转换器。

#### 14. ApiKey API 密钥（/apiKey）— 复杂度 MEDIUM

| 端点 | 行为 | 易丢失细节 |
|------|------|-----------|
| GET /apiKey/list | 分页 createTime 倒序 | — |
| POST /apiKey/add | 生成 key = `UUID 去连字符`（32 位十六进制）；userId 可选但非空时必须存在（冗余写 username）；status 默认 ENABLED；**返回完整 ApiKey 含明文 key**（一次性展示语义） | — |
| PUT /apiKey/status | 仅改状态（ENABLED/DISABLED） | 停用立即生效（登录时仓储过滤） |
| DELETE /apiKey/delete/{id} | 物理删除 | — |

登录校验（仓储层）：`apiKey 精确匹配 + status=ENABLED + (expireTime IS NULL OR expireTime > now)`。

#### 横切机制 A：名称冗余同步（6 个领域事件监听器）— 复杂度 HIGH

冗余字段非规范化设计，改名必须级联同步，否则测试不过：

| 事件（触发点） | 同步目标 |
|---------------|---------|
| DeptNameChangedEvent（PUT /dept/edit） | User.deptName（按 deptId）+ 子 Dept.parentName（按 parentId） |
| RoleNameChangedEvent（PUT /role/edit） | RoleMenu.roleName + UserRole.roleName |
| MenuNameChangedEvent（PUT /menu/edit） | RoleMenu.menuName + 子 Menu.parentName |
| DictTypeNameChangedEvent（PUT /dictType/edit） | DictData.dictTypeName |
| UserRealNameChangedEvent（PUT /user/edit，realName 变化） | **14 张表**：全部业务表的 createByName/updateByName（按 createById/updateById）+ OperationLog.realName + UserRole.username |
| UsernameChangedEvent | 仅 UserRole.username（HTTP 层无触发点——edit 不可改 username；为将来保留） |

**关键移植语义**：所有 UPDATE 带 `!= newName` 条件（避免无谓写）；RealNameSyncListener 用
**AFTER_COMMIT + REQUIRES_NEW**（主事务提交后执行，修复同线程自死锁 —— 源 2026-09-29 生产实测教训）；
其余 4 个监听器为事务内 `REQUIRES_NEW`。Go 版事件机制（进程内 channel/观察者）需保留此执行时序差异。

#### 横切机制 B：审计字段自动填充 — 复杂度 MEDIUM

- BaseAggregate：id（雪花）、createTime、updateTime、remark
- BaseUserEntity（业务表继承）：createById、createByName、updateById、updateByName
- insert 填：createTime + updateTime + 4 个审计字段；update 填：updateTime + 2 个更新人字段
- 操作人来自请求级上下文（源：Sa-Token 过滤器注入 ThreadLocal UserContext；Go：Gin middleware + context.Context）
- 无登录上下文（后台任务）时**跳过填充保持 null**（语义：系统行为无操作人）
- Go 版需等价实现：GORM 钩子（BeforeCreate/BeforeUpdate）+ context 注入

#### 横切机制 C：认证与鉴权设施 — 复杂度 HIGH

- 全局登录过滤器：白名单路径免登录（源为 Nacos craft-satoken.yaml 配置路径列表；Go 版改本地 YAML），其余全部 checkLogin
- `@SaCheckPermission("system:user:list" 等)` 注解式权限：权限字符串约 50 个常量，格式 `{模块}:{业务}:{操作}`
- 权限/角色数据从**会话快照**读取（StpInterface 实现）
- super_admin 角色编码豁免：登录时加载全部菜单权限
- 错误映射：未登录→401 NEED_LOGIN、无权限→403 ACCESS_DENIED
- UserContext 注入（beforeAuth 阶段）+ 请求结束清理（防线程池残留）
- 会话存 Redis（sa-token-redis-jackson）——Go 版保留 Redis 会话（PROJECT.md 已定）

#### 横切机制 D：统一响应与异常 — 复杂度 LOW

- `R{code, msg, data, total?}` 包装；分页端点返回 `records + total`
- 异常映射（Go 版恢复为 error 处理 + 统一 handler）：IllegalArgument→400（ILLEGAL_ARGS 带消息）、
  参数校验→400（字段错误拼接）、类型不匹配→400、缺参→400、未登录→401、无权限→403、兜底→500 固定文案（防信息泄漏）
- 分页默认：page=1 size=15，小于 1 钳位（LogQuery 例外：1/10）

### Differentiators（项目特有行为 —— 必须复刻但非通用 IAM）

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| 排班管理 Shift | WCS/MES 工厂班次定义，供排产/设备系统消费 | LOW | 源内就只有 CRUD，全量无分页 |
| 故障库 FaultInfo + Excel 三键 upsert 导入 | 设备故障知识库，PLC 位号级（name+bitIndex+location） | MEDIUM | ImportResult{insertCount,updateCount} 契约 |
| API Key 登录通道 | 第三方系统以 key 换 token 调用 IAM 保护接口 | MEDIUM | 与管理端 ApiKey CRUD 联动 |
| 文件分类树 + 内容 MD5 判重 | 工控现场文档/图纸管理 | MEDIUM | IS NULL 判重分支是隐藏坑 |
| 名称冗余同步事件网 | 查询性能换一致性（全部列表页直接显示名称免 join） | HIGH | AFTER_COMMIT 时序防死锁 |
| Excel 导出（日志/故障库，中文表头） | 现场报表 | MEDIUM | Go 选型 excalitech/excelize |

### Anti-Features（确认不移植）

| Feature | Why Requested（源里为何存在） | Why Problematic | Alternative（Go 版处置） |
|---------|------------------------------|-----------------|--------------------------|
| **OAuth2 授权码流程** | build.gradle.kts 引入 `cn.dev33:sa-token-oauth2:1.45.0` | **专项验证结论：全仓库（sys-iam + sys-kernel）零 SaOAuth2 import、零端点、零配置引用 —— 纯死依赖** | 不实现。登录即 token（Sa-Token 风格）已满足前端 |
| Dubbo RPC 4 接口（User/DictData/Shift/FaultInfo） | 微服务间调用（craft-wcs/wms 消费） | 单机部署无服务间调用（PROJECT.md Out of Scope） | 不移植。注意 UserRpcServiceImpl 含 WMS 用户同步逻辑（syncWmsUsers、默认密码 yf175@123456、openid 绑定、extToken 维护）一并去除 |
| Nacos 配置/注册中心 | 中心化集群部署 | 工控机单机无注册发现需求 | 本地 YAML + 环境变量 |
| dataScope 数据权限过滤 | Role.dataScope 字段存在 | **源只有存储字段，零过滤实现**（grep 全仓库确认仅 CRUD 存取） | schema 沿用故字段保留；不实现任何过滤逻辑 |
| X-API-Key 请求头认证 | SessionKeys.API_KEY_HEADER 常量已定义 | 无任何消费方（API Key 仅 POST body 方式） | 不实现 header 方式；保留 body 端点 |
| WMS 用户同步仓储方法 | createForWmsSync / findByOpenid / listByOpenids / updateExtToken | 依赖 Dubbo RPC 场景 | 随 RPC 去除（schema 列保留不动） |
| 前端favicon/静态资源 | Spring 静态目录 | Go 服务无此需要 | 忽略 |

## Feature Dependencies

```
[Auth 认证]
    ├──requires──> [User]（登录主体）
    ├──requires──> [Role]（角色码 → super_admin 豁免判断）
    ├──requires──> [Menu]（权限串 + 路由树）
    └──requires──> [ApiKey]（apikeyLogin）

[User] ──requires──> [Dept]（deptId 外键校验 + deptName 冗余）
[User] ──requires──> [Role]（角色分配 user_role）
[Role] ──requires──> [Menu]（菜单分配 role_menu）
[DictData] ──requires──> [DictType]
[File] ──requires──> [FileCategory]（分类校验 + 判重）
[FaultInfo 导入导出] ──requires──> [Excel 设施]
[OperationLog 切面] ──requires──> [Auth 会话]（取 userId/realName）+ [OperationLog 存储]
[审计字段填充] ──requires──> [Auth 上下文]（Gin middleware 注入）
[名称同步监听器] ──requires──> [Dept/Role/Menu/DictType/User 全部就绪]（跨域事件，最后接线）
[ApiKey] ──requires──> [User]（userId 校验）
```

### Dependency Notes

- **Auth 必须排在 User/Role/Menu/ApiKey 之后**：登录要读四域数据装配会话
- **名称同步是最后接线项**：6 个监听器横跨 8 个域，建议各域 CRUD 稳定后统一实现+测试
- **操作日志切面依赖 Auth**：无会话时字段留空但不报错（匿名可访问白名单路径也产生日志）
- **Excel 设施先于 FaultInfo**：导入导出共用一套注解/映射方案（Go: excelize 封装）

## MVP Definition

本项目为全量移植，无"精简 MVP"概念 —— 验收即 14 域全过。以下为**实现顺序建议**（供 roadmap 参考）：

### Launch With（v1 = 全量）

- [ ] 工程骨架 + R 响应/异常/分页/审计填充四件套（所有域的地基）
- [ ] Auth + User + Role + Menu + Dept（RBAC 主链路，含会话快照语义）
- [ ] DictType/DictData + Config + ApiKey（配置类域）
- [ ] FileCategory + File（存储抽象：本地磁盘先行，RustFS S3 第二实现）
- [ ] Shift + FaultInfo（含 Excel 导入导出）
- [ ] OperationLog 切面（Gin 中间件）
- [ ] 名称同步监听器（AFTER_COMMIT 等价时序）

### Add After Validation（v1.x）

- [ ] OpenAPI/Swagger 文档（PROJECT.md Active 项，不阻塞验收主链路）
- [ ] 健康检查端点（等价 actuator health + Prometheus 指标）

### Future Consideration（v2+，明确不进本期）

- [ ] Dubbo RPC 等价物（协议选型再议）
- [ ] OAuth2 授权码流程（源就是死依赖）
- [ ] dataScope 数据权限（源未实现）
- [ ] 禁用用户踢会话、密码强度、账号锁定（源均无；如需属于新需求非移植）

## Feature Prioritization Matrix

| Feature | User Value | Implementation Cost | Priority |
|---------|------------|---------------------|----------|
| 横切四件套（R/异常/分页/审计） | HIGH | MEDIUM | P1 |
| Auth（含会话快照 + apikeyLogin） | HIGH | HIGH | P1 |
| User（含 UserVo 防N+1 + builtin 守卫） | HIGH | HIGH | P1 |
| Role / Menu / Dept | HIGH | MEDIUM | P1 |
| DictType / DictData / Config | MEDIUM | LOW | P1 |
| OperationLog（切面+查询+导出） | HIGH | MEDIUM | P1 |
| FileCategory / File（含判重/MD5） | HIGH | HIGH | P1 |
| ApiKey | MEDIUM | LOW | P1 |
| Shift | MEDIUM | LOW | P1 |
| FaultInfo（含 Excel upsert） | MEDIUM | MEDIUM | P1 |
| 名称同步监听器 ×6 | HIGH | HIGH | P1 |
| Excel 导出设施 | MEDIUM | MEDIUM | P1 |
| OpenAPI 文档 / 健康检查 | MEDIUM | LOW | P2 |

## Competitor Feature Analysis（移植基准对照 —— 源实现 vs Go 版应采用）

| Feature | 源实现（Java） | Go 版处置建议 |
|---------|---------------|---------------|
| 会话/token | Sa-Token + Redis（sa-token-redis-jackson） | 等价自建：Redis 存会话 + middleware 校验（PROJECT.md 已定保留 Redis） |
| 权限注解 | @SaCheckPermission 读会话快照 | Gin middleware + 权限串常量表（50 个 `system:*:*` 语义保留） |
| BCrypt 密码 | hutool BCrypt | `golang.org/x/crypto/bcrypt`（与存量密码哈希兼容，可直接校验旧库密码） |
| 领域事件 | Spring @EventListener / @TransactionalEventListener | 进程内事件总线（channel 或观察者注册表），保留 AFTER_COMMIT 语义 |
| 审计填充 | MyBatis-Plus MetaObjectHandler | GORM BeforeCreate/BeforeUpdate 钩子 |
| 雪花 ID | MP ASSIGN_ID | 沿用 schema BIGINT；Go 雪花库或继续由 DB 侧兼容（自增亦可，但存量数据已是雪花值） |
| 对象存储 | starter-rustfs（S3 协议） | 存储接口抽象：本地磁盘 + S3（minio-go）双实现可配置 |
| Excel | Apache Fesod（EasyExcel 系） | excelize（注解等价用 struct tag + 映射器封装） |
| 参数校验 | jakarta.validation | binding tag（gin）+ validator |
| 树构建 | 内存建树（parentId 0/null 根，孤儿提升） | 同算法复刻（含孤儿提升语义，测试会测） |
| 枚举存储 | @EnumValue int 值 | 常量 + GORM 自定义类型（schema 列为 int，零迁移） |

## Sources（本次研究全部为一手源码阅读，置信度 HIGH）

- 控制器 ×14：`/home/wx/workspace/sys-iam/sys-iam/src/main/java/com/wxinfo/system/iam/interfaces/controller/`
- 应用服务 ×14：`.../iam/application/service/`
- 操作日志切面：`.../iam/application/listener/OperationLogAspect.java`
- 名称同步监听器 ×5 文件（6 事件）：`.../iam/application/listener/*SyncListener.java`
- 实体 ×13：`.../iam/domain/entity/` + `sys-kernel/.../kernel/domain/entity/`（BaseAggregate/BaseUserEntity/DictType/DictData/FaultInfo）
- 仓储实现 ×13：`.../iam/infrastructure/persistence/`
- 公共设施：`sys-kernel/.../kernel/infrastructure/config/`（SaTokenConfig/SaWhiteListProperties/AutoFillHandler/GlobalExceptionHandler）、`shared/constant/SystemPermissions.java`、`shared/context/UserContext.java`、`infrastructure/persistence/PageHelper.java`
- OAuth2 验证：`grep -r SaOAuth2 全仓库` 零命中；依赖声明在 `sys-iam/build.gradle.kts` 与根 `build.gradle.kts`（sa-token-oauth2:1.45.0）
- Excel：`common-utils/.../utils/excel/ExcelHelper.java`（Apache Fesod）
- 源单元测试 ×7（行为锚点）：RoleRemoveTest、FileUploadHashDedupTest、BuiltinFieldMappingTest、RoleQueryTest、UserBuiltinGuardTest、UserApplicationServiceGenderVoTest、GenderTest

---
*Feature research for: sys-iam Java→Go 全量移植*
*Researched: 2026-10-31*
