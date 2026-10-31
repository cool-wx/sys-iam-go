# Pitfalls Research

**Domain:** Java (Spring Boot + MyBatis-Plus + Sa-Token) → Go (Gin + GORM) IAM 后端整体移植，MySQL schema 原样沿用、Redis 会话保留、工控机单机部署
**Researched:** 2026-10-31
**Confidence:** HIGH（全部关键结论均有源码直接证据 + 官方文档核对；个别标 MEDIUM 处注明信息缺口）

> 证据来源：`/home/wx/workspace/sys-iam` 源码逐文件审读（kernel 基础设施、13 个仓库实现、
> User/Auth 应用服务、操作日志切面、事件监听器、ExcelHelper、Redis 序列化器），外部核对
> GORM/bcrypt/excelize 官方文档与 sa-token 社区资料。下文引用源文件均给出路径。

## Critical Pitfalls

### Pitfall 1: 雪花主键必须由 Go 应用层生成（DB 无 AUTO_INCREMENT）

**What goes wrong:**
源项目所有实体 `@TableId(type = IdType.ASSIGN_ID)`（见 `sys-kernel/.../domain/entity/BaseAggregate.java:30`），ID 由 MyBatis-Plus 在应用内生成（雪花算法，约 19 位 int64）。MySQL 列是普通 `BIGINT`，**没有 AUTO_INCREMENT**。Go 移植若按 GORM 惯例不赋 ID 直接 `Create()`：
- 列为 NOT NULL 且无默认值 → INSERT 直接报错 `Field 'id' doesn't have a default value`；
- 若模型误标 `gorm:"autoIncrement"` → 与既有存量数据段冲突或建表校验失败。

**Why it happens:**
开发者默认"数据库管主键"，而 MP 的 ASSIGN_ID 是应用层行为，schema 看不出来。

**How to avoid:**
- 引入 Go 雪花库（如 `bwmarrin/snowflake` 或 `sony/sonyflake`），在 GORM `BeforeCreate` hook 或统一 Base 模型中赋值 ID；
- 雪花 epoch/workerId 不影响正确性（只需全局唯一），workerId 用单机固定值即可；
- 单元测试锁定：`Create` 后 ID 非零且 > 2^53。

**Warning signs:**
首个 INSERT 测试报 "Field 'id' doesn't have a default value"；或插入后 `SELECT MAX(id)` 远小于存量数据位数。

**Phase to address:**
数据访问层阶段（kernel 等价层）——所有表共享，必须在任何业务表落库前解决。

---

### Pitfall 2: int64 ID 必须 JSON 字符串化（前端精度丢失）

**What goes wrong:**
雪花 ID 是 19 位，超过 JS `Number.MAX_SAFE_INTEGER`（2^53-1，约 16 位）。源项目用全局 Jackson `Long → String` 序列化规避（`sys-kernel/.../config/JacksonConfig.java:88-90`，注释明确写"防 JS 端 2^53 精度丢失"）。Go 端 `encoding/json` 默认把 int64 输出为数字 → 前端拿到 `14219512101927813000` 这类被舍入的 ID，回查/更新/删除全部错位。

**Why it happens:**
Go 没有全局序列化定制点，逐字段忘加 tag 是常态；且"API 允许重新设计"容易被误读为"可以不管 ID 精度"。

**How to avoid:**
- API 无论怎么重设计，ID 字段统一输出字符串：自定义 `type ID int64` 实现 `MarshalJSON`/`UnmarshalJSON`，或 DTO 层一律 `string`；
- 入参方向同样兼容：gin binding 接受字符串数字，统一在 DTO 解析（Jackson 原本就同时接受 `"5"` 与 `5`）；
- 在 API 骨架阶段写一条契约测试：任意实体 JSON 输出中 id 为字符串。

**Warning signs:**
浏览器 console 里拿到的 id 末尾是 000；按 id 更新报"用户不存在"。

**Phase to address:**
API 骨架阶段（响应包装/序列化约定与源系统对齐时）。

---

### Pitfall 3: GORM 零值更新 ≠ MyBatis-Plus 的 null-skip（最隐蔽的数据破坏点）

**What goes wrong:**
源系统大量依赖 MP `updateById` 的"只更新非 null 字段"语义做**部分更新**：
- `UserRepositoryImpl.updateStatus()` 只 new 一个 id+status 的实体（`UserRepositoryImpl.java:86-92`）；
- `editUser → User.withEdit(...)` 后直接 `updateById`，请求里没传的字段（null）不会写库（`UserApplicationService.java:131-136`）；
- `User.withEdit` 注释明确："仅显式传入时覆写 builtin，编辑请求省略该键不得清掉已有内置标志"。

Go 三重错位：
1. `gorm.Save()` 是**全字段覆盖**——未提交字段被写成零值，直接清空生产数据；
2. `Updates(struct)` 跳过零值（`false`/`0`/`""`）——与 Java 的"跳过 null"形式相似但语义不同：Go 的 string/bool **没有 null**，无法区分"未提供"与"清空为空串"，`firstLogin=false`、`visible=false`、清空 phone 这类合法更新会被静默吞掉；
3. 请求 DTO 若用值类型接收 JSON，`"phone": ""` 与缺省不可区分。

**Why it happens:**
Go 缺 nullable 语义，移植者按 Java 形状直译结构体；GORM 文档的零值陷阱不读不会知道。

**How to avoid:**
- 写路径 DTO 一律用指针字段（`*string`/`*bool`/`*int64`），nil=未提供，有值=覆写（包括显式 null→按需定义为清空）；
- 更新统一走 `Updates(map[string]interface{})` 或 `Select(显式列)`，禁止裸 `Save()` 于更新路径；
- 为"部分更新"写一个共享 helper（如 `Patch(m map[string]any)`），所有仓库实现复用；
- 契约测试：`editUser` 场景断言未提交字段保持原值、显式空串可清空。

**Warning signs:**
编辑用户后头像/邮箱莫名变空；`builtin` 标志被重置；测试里断言 `firstLogin=false` 更新失败。

**Phase to address:**
数据访问层阶段定 helper 约定 + 各功能域阶段逐一落实；RBAC（用户/角色/菜单）阶段验收必须覆盖。

---

### Pitfall 4: 不要给 GORM 模型加 DeletedAt（源系统是硬删除）

**What goes wrong:**
移植者按 Go 社区模板给模型加 `gorm.DeletedAt`（软删除是 GORM 教学默认）。但源系统**全库无 `@TableLogic`**，`removeById` 是物理 DELETE，schema 里根本没有 `deleted_at` 列。加上后 GORM 会在每条 SELECT 拼 `WHERE deleted_at IS NULL` → 立即 `Unknown column 'deleted_at'`，或更糟：迁移时不报错、在运行期才炸。

**Why it happens:**
"ORM 最佳实践"直觉与既有 schema 冲突；schema 复用约束下不能引入软删除。

**How to avoid:**
- 模型不带 `DeletedAt`；删除语义照源=硬删除；
- 若未来要软删除，须改 schema，属于显式决策而非顺手行为；
- 内置用户保护逻辑已在服务层（`UserApplicationService.removeById` 的 builtin 硬拦截），移植保留在服务层而非数据库层。

**Warning signs:**
查询报 unknown column；或 code review 见到 `gorm.DeletedAt` 字样。

**Phase to address:**
数据访问层阶段（模型定义约定中明文禁止）。

---

### Pitfall 5: JSON 序列化契约三特征漂移（Long→String / 时间格式 / null 键）

**What goes wrong:**
源 REST 面有全局锁定契约（`JacksonConfig.java`，且有专门的 `JacksonConfigTest` 锁定）：
1. Long 全部输出字符串（见 Pitfall 2）；
2. `LocalDateTime` 输出 `yyyy-MM-dd HH:mm:ss`（空格分隔、**无 T 无时区**），Date/LocalDate/LocalTime 三个 pattern 同理；
3. null 字段**照常输出键**（不用 `omitempty`）；
4. 枚举写侧输出 **name**（`"ENABLED"`），读侧兼容数字与字符串（`EnumByValueJacksonModule.java`）；
5. 读侧关闭未知字段报错（FAIL_ON_UNKNOWN_PROPERTIES=false，宽容输入）。

Go 默认：`time.Time` → RFC3339（`2026-01-02T15:04:05+08:00`），int64 → 数字，零值字段 `omitempty` 惯用。全量漂移。

**Why it happens:**
Go `encoding/json` 无全局定制，习惯性 `omitempty`；API 重设计的自由度让人忽略既有客户端/测试/导出工具对格式的依赖。

**How to avoid:**
- 建"序列化约定"作为 API 骨架的一等公民：自定义 `LocalDateTime` 等价类型（`MarshalJSON` 固定 `"2006-01-02 15:04:05"` 格式 + Asia/Shanghai），枚举类型实现 `MarshalJSON`（输出 name）与 `UnmarshalJSON`（收 name 或 int，未知值按源语义兜底默认值）；
- 响应 DTO 禁止 `omitempty`（保持 null 键输出）；
- 把源 `JacksonConfigTest` 的断言逐条翻成 Go 契约测试。

**Warning signs:**
前端时间控件解析失败；测试断言里出现 `T00:00:00Z`；未知枚举值 500 而非兜底。

**Phase to address:**
API 骨架阶段（统一响应包装同时建立）；各功能域阶段用契约测试回归。

---

### Pitfall 6: 时区漂移——LocalDateTime 墙钟语义 vs Go time.Time 带时区

**What goes wrong:**
- 源全链路是"墙钟"语义：`AutoFillHandler` 写 `LocalDateTime.now()`（JVM 默认时区），MySQL DATETIME 无时区，Jackson 输出无时区字符串；
- Go `time.Time` 永远带 location。DSN 若缺 `loc` 参数（默认 UTC），读取 DATETIME 差 8 小时；序列化 RFC3339 又把时区暴露出去，和 Pitfall 5 叠加；
- 工控机部署的系统时区不可假设（可能是 UTC），Java 版靠容器/镜像 TZ 约定，Go 版必须显式。

**Why it happens:**
开发机（中国时区）上一切正常，loc 缺省只在 UTC 环境或部署机复现——典型"上线才炸"。

**How to avoid:**
- DSN 固定：`charset=utf8mb4&parseTime=True&loc=Asia%2FShanghai`（loc 值必须 URL 编码）；
- systemd unit 显式 `Environment=TZ=Asia/Shanghai` 或代码内 `time.LoadLocation` 固定业务时区，不依赖机器 TZ；
- 时间在 DTO 层输出无时区字符串（Pitfall 5 类型一并解决）；
- 集成测试在 `TZ=UTC` 环境跑一遍，锁定无 8 小时漂移。

**Warning signs:**
列表页创建时间差 8 小时；凌晨新建数据"提前 8 小时"出现在前一天。

**Phase to address:**
工程骨架阶段（DSN/配置模板）+ API 骨架阶段（序列化）。

---

### Pitfall 7: BCrypt 互操作——hash 兼容没问题，错误语义才是坑

**What goes wrong:**
源用 hutool `BCrypt.checkpw/hashpw`（jBCrypt 系，`$2a$`、cost 10；`AuthApplicationService.java:56`、`UserApplicationService.java:103`）。Go `golang.org/x/crypto/bcrypt` 可直接验证存量 `$2a$` hash（截断规则同为 72 字节），**存量密码全部可登录**——这点是安全的。真正的坑在错误语义：
- Java：hash 畸形 → `checkpw` 返回 false（或抛异常被上层翻译成"用户名或密码错误"）；
- Go：`CompareHashAndPassword` 对畸形/过短 hash 返回 **error**——移植者常把 error 当 500 处理，畸形存量数据导致登录接口 500；
- Go `GenerateFromPassword` 对 >72 字节密码返回 error，Java 则静默截断——超长密码注册时行为分叉。

**Why it happens:**
Java 布尔返回 vs Go (ok, error) 惯例差异被忽略；存量 DB 里可能有历史脏 hash。

**How to avoid:**
- 封装 `VerifyPassword(hash, plain) bool`：任何 error 一律返回 false（登录失败），error 细分仅记日志；
- 新密码用 `GenerateFromPassword(plain, bcrypt.DefaultCost)`（=10，与源一致），产生 `$2a$`；
- 不做存量 hash 重迁移（保持 `$2a$` 原样）；
- 上层统一报"用户名或密码错误"，禁止区分"用户不存在/密码错"（源 `AuthApplicationService.login` 即如此）；
- 一次性脚本抽样验证：从生产库取 20 个 hash 用 Go 验证通过后再上线。

**Warning signs:**
个别老账号登录 500；日志出现 `crypto/bcrypt: hashedSecret too short`。

**Phase to address:**
认证阶段（登录/重置密码实现点）；数据兼容验收清单必含"存量密码可登录"。

---

### Pitfall 8: 自动填充审计字段等价实现（AutoFillHandler + BlockAttack 双件套）

**What goes wrong:**
两处 MP 全局行为在 GORM 没有开箱等价物：
1. `AutoFillHandler`（`config/AutoFillHandler.java`）：insert 填 createTime/updateTime/createById/createByName/updateById/updateByName；update 填 updateTime/updateById/updateByName；操作人来自 `UserContext`（ThreadLocal），**无登录上下文时跳过填充（保持 NULL，语义="系统行为无操作人"）**。GORM 惯例 `CreatedAt/UpdatedAt` 字段名与本 schema（`create_time/update_time`）不符，`autoUpdateTime` 只管时间不管审计人；
2. `BlockAttackInnerInterceptor`（防全表 update/delete，`MybatisPlusConfig.java:27`）：GORM **无等价插件**，一条漏掉 `Where` 的全局 Update 就是全表事故。

**Why it happens:**
ThreadLocal→Go 无直接对应；GORM hooks 需要 context 传递链；BlockAttack 是隐性安全网，没了没人发现直到出事。

**How to avoid:**
- 统一 Base 模型 + `BeforeCreate`/`BeforeUpdate` hooks：时间用 `time.Now()`（业务时区），审计人从 gin `*gin.Context` → `context.Context` 传递（中间件注入 userId/userName）；
- 无用户上下文（后台任务）时跳过审计字段、仍填时间——与源语义逐字对齐；
- 全表防护用约定+测试替代：仓库层只暴露带条件的更新封装；CI 里 grep `db.Model(...).Update(` 后无 `Where` 的模式；
- 更新路径统一走共享 helper（Pitfall 3），hook 挂在 helper 上而不是每个模型。

**Warning signs:**
列表页"创建人"列空白（后台路径误填或漏填）；code review 见裸全局 Update。

**Phase to address:**
数据访问层阶段（kernel 等价层实现）；操作日志阶段复用同一上下文注入。

---

### Pitfall 9: 名称同步监听器的事务与执行语义（Spring 事件 → Go）

**What goes wrong:**
6 个名称冗余同步监听器（Dept/DictType/Menu/Role/RealName，如 `DeptNameSyncListener.java`）是 `@EventListener` **同步**执行 + `@Transactional(REQUIRES_NEW)` 独立事务。移植常见错误：
1. 改成 goroutine 异步 → editUser 返回后立即查询，冗余字段（user.deptName）还是旧值，测试随机失败；进程退出时事件丢失；
2. 放进外层事务 → 与源行为不同（源是独立事务：外层回滚同步仍生效，存在既定不一致窗口）；
3. 忽略 `ne(条件)` 的 NULL 语义：`.ne(User::getDeptName, newName)` 在 MySQL 里 `NULL <> 'x'` 不为真，**deptName 为 NULL 的行不会被更新**——Go 用 GORM 写 `Where("dept_name <> ?", x)` 行为一致，但"顺手修 bug"加 `OR dept_name IS NULL` 就改变了行为。

**Why it happens:**
Spring 事件看起来像消息队列，被异步化；REQUIRES_NEW 的事务边界在 Go 没有注解，需要手写 db.Transaction。

**How to avoid:**
- Go 版监听器=普通同步函数调用（同 goroutine），事务边界显式：监听器内部开独立 `db.Transaction(...)`；
- 事件定义为一组强类型 struct + 一个 `EventBus.Publish`（进程内、同步、无队列）；
- `.ne` 条件原样翻译并加注释锁定 NULL 行为；测试断言 NULL 行不更新；
- 超级管理员短路逻辑（`AuthApplicationService` 里 roleCodes 判断）与事件无关但同文件，别漏。

**Warning signs:**
改部门名后用户列表 deptName 没变（异步丢事件）或测试偶发失败（竞态）。

**Phase to address:**
事件/名称同步阶段（依赖 RBAC 主体完成后的收尾阶段）。

---

### Pitfall 10: Redis 会话语义——key 结构可换，TTL/刷新语义不能丢

**What goes wrong:**
源用 sa-token-redis-jackson（`sys-iam/build.gradle.kts` 依赖确认），Redis 里是 `satoken:login:token:{tokenValue}`→loginId、`satoken:login:session:{loginId}`→SaSession JSON（含 permissions/roles/username/realName 四键，`AuthApplicationService.loadSessionData`）。**存量会话不迁移**（PROJECT.md Out of Scope），所以 Go 可以自选 key 结构；容易丢的是语义：
- 权限快照在**登录时**写入会话，之后改角色不刷新——必须重新登录才生效（`loadSessionData` 只在登录路径调用）。Go 版若改成每请求查库取权限，行为"更好"但压测/安全评审基线变了；若做缓存失效则复杂度暴涨；
- `refreshToken` = `StpUtil.renewTimeout(timeout)`（把 TTL 重置为 timeout，`AuthApplicationService.java:122-126`）；
- API Key 登录：若该用户已有带 permissions 的会话则直接复用 `StpUtil.login`，否则走完整加载（`loginByApiKey`）；
- 超时值（timeout / active-timeout）配置在 **Nacos craft-satoken.yaml，仓库里没有**——移植者拿不到数值。

**Why it happens:**
"等价能力"被理解成"重新设计会话模型"；配置不在代码仓库，默认值被拍脑袋。

**How to avoid:**
- 保持"登录时快照权限入会话"模型，Redis 结构自选（建议 hash：token→loginId 带 TTL，session JSON 带 TTL）；
- 向用户索取 Nacos 里的 `sa-token.timeout/active-timeout` 实际值，写进 Go 配置默认值（MEDIUM 置信：本仓库不可见）；
- token 续期接口保持"重置为完整 timeout"语义；
- 会话存储用 `go-redis` SET+TTL 实现，登出=DEL token+session（源 logout 全删）；
- 兼容性验收：重启 Go 服务后 Redis 里旧 satoken: 键变成死键——上线 runbook 加一条"切换前 FLUSHDB 会话库或确认 TTL 自然过期"（停机切换，PROJECT.md 已定）。

**Warning signs:**
改角色后不重新登录就有新权限（说明权限走了实时查询）；refreshToken 后 TTL 没变满。

**Phase to address:**
认证阶段；配置值确认放在工程骨架阶段的配置清单里。

---

### Pitfall 11: 操作日志切面移植——gin 中间件的参数捕获与优雅退出

**What goes wrong:**
`OperationLogAspect`（`application/listener/OperationLogAspect.java`）对**所有 controller 方法**记录：userId/realName/ip/method+URI/module(类名)/operation(方法名)/前 3 个非 request/response/multipart 参数的 JSON（敏感字段掩码）/status 1|0/errorMsg 截 500/duration(ms)，在 finally 里同步落库，落库失败吞掉。Go 移植常见错误：
1. gin 默认读完 body 即弃——中间件要 `io.ReadAll` + 回放 body，漏回放则 handler 拿不到请求体；
2. module/operation 来自 Java 类名/方法名（反射），Go 没有对应物——要么用路由 pattern、要么显式注解 handler，随手用 `c.FullPath()` 会导致日志可读性下降、与存量日志查询习惯断裂；
3. 敏感字段掩码是正则匹配 JSON 文本（password/oldPassword/newPassword/secretKey/apiKey/token/accessToken/refreshToken → `******`），Go 若在结构体层做 tag 掩码，字段覆盖面和源不一致；
4. 中间件里起 goroutine 写日志 → 进程重启丢日志、panic 无人 recover 直接崩进程。

**Why it happens:**
AOP 的"对所有方法无感生效"在 Go 需要显式中间件注册，细节容易走样。

**How to avoid:**
- 单个 gin 中间件包 `c.Next()`，body 用 `io.ReadAll` 后 `c.Request.Body = io.NopCloser(bytes.NewBuffer(...))` 回放， multipart 请求跳过读 body（源就排除 MultipartFile）；
- module/operation：注册路由时带显式元数据（如 `r.POST("/user", h, logMeta("UserController", "addUser"))`），迁移期与源类名/方法名对齐，保证存量日志可对照；
- 掩码直接复用同一正则对序列化后的 JSON 文本做替换（和源逐字一致）；
- 日志写异步化（带缓冲 channel + 单写 goroutine + SIGTERM flush），或先同步保真、量上来再异步——单机低并发同步完全可接受；
- errorMsg 截 500、duration 取整 ms、status 失败=0 成功=1，逐条对齐。

**Warning signs:**
POST 接口 400（body 被中间件吃掉）；日志表 params 全空或含明文密码。

**Phase to address:**
操作日志阶段（依赖认证上下文注入，Pitfall 8 同一套 context）。

---

### Pitfall 12: Excel 导入——表头映射、单元格类型、未知值兜底

**What goes wrong:**
源用 fesod（EasyExcel fork，`common-utils/excel/ExcelHelper.java`），**按中文表头文本匹配字段**（`@ExcelProperty("用户名")`），性别列是中文（男/女/未知，`GenderConverter.fromInfo` 兜底 UNKNOWN），状态列同构（`UsageStatusConverter`/`fromValue` 未知值兜底 ENABLED，`UsageStatus.java:26-31`）。excelize 没有注解映射，移植常见错误：
1. 按列序号硬编码映射 → 用户模板增删一列全错位；
2. `GetCellValue` 拿到的数字单元格是格式化字符串（`123456.0`、手机号 `1.23E+10` 科学计数法）→ bitIndex/手机号解析错误；
3. excelize `Rows.Columns()` **不跳过尾部空行**，且 v2.5.0 起必须 `rows.Close()` + `f.Close()`，漏 Close 在长运行进程里泄漏临时文件/内存（Pitfall 16 关联）；
4. 源导入是 `@Transactional` 整批（`FaultInfoApplicationService.importExcel`），逐行 name==null continue、按 name+bitIndex+location 查重决定 insert/update，返回 `ImportResult{insertCount,updateCount}`——Go 若逐行独立事务，半批成功的结果与源不同。

**Why it happens:**
EasyExcel 的声明式映射被手工循环替代，细节（类型转换、空行、兜底值）散落丢失。

**How to avoid:**
- 写一个按表头文本→字段的小映射器（读首行建 map），保留中文表头为契约；
- 单元格读取统一走 `GetRawCellValue`/`GetCellType` 判型后清洗（去 `.0`、反科学计数法）；
- 枚举解析保留"未知值兜底默认"语义（封装 `FromInfo`/`FromValue` 函数）；
- 导入整体包一个 `db.Transaction`，逐行查重逻辑照搬，结果结构 insertCount/updateCount 保留；
- 用源系统的真实导出文件做 fixtures 回归（用户模板、故障库模板各一份）。

**Warning signs:**
导入后性别/状态全变默认值；手机号显示 1.23456E+10；导入失败但库里出现半批数据。

**Phase to address:**
Shift/FaultInfo/用户导入功能域阶段（Excel util 属该阶段前置任务）。

---

### Pitfall 13: 文件下载全量进内存 + 上传内存阈值（工控机内存放大）

**What goes wrong:**
源 `FileController.download` 返回 `ResponseEntity<ByteArrayResource>`——整个文件读成 `byte[]`（`FileController.java:62-65`）。中心服务器内存大没炸；工控机（常见 4-8GB）上大文件 + 两三个并发下载就能 OOM。上传侧 MultipartFile 默认全缓冲，gin 的 `MaxMultipartMemory` 默认 32MB，大附件直接失败或暴涨内存。

**Why it happens:**
Java 侧 byte[] 顺手写法被原样翻译；Go 的优势（io.Reader 流式）没被利用。

**How to avoid:**
- 下载改流式：本地盘 `http.ServeFile`/`io.Copy`，S3(RustFS) 用 SDK 的 GetObject 返回 Reader 直通 `c.Writer`，边读边写；
- 上传：`c.FormFile` + `SaveUploadedFile` 或直接流式读 `multipart.File`，`MaxMultipartMemory` 调到 8-16MB；
- 这是**行为增强**（API 响应流式化），属允许重设计范围，但要写进 API 变更说明。

**Warning signs:**
压测下载大文件时 RSS 涨到 GB 级不回落；上传 >32MB 报错。

**Phase to address:**
文件功能域阶段（本地磁盘/RustFS 抽象设计时一并定流式接口）。

---

### Pitfall 14: 连接池默认值在工控机上双双过大或过小

**What goes wrong:**
- GORM 底层 `database/sql` 默认 `MaxOpenConns=0`（无限）。MySQL 默认 `max_connections=151`；操作日志中间件同步写库 + 业务查询 + Excel 导入循环查重叠加，单机并发一高连接耗尽，全部请求卡 `dial tcp` 超时；
- go-redis 默认 `PoolSize = 10 × GOMAXPROCS`，工控机 CPU 核少问题不大，但若误配大值会占满 Redis `maxclients`；
- 源 Java 侧 HikariCP 默认 10 连接——Go 版若不设，行为基线直接翻倍以上。

**Why it happens:**
单机低并发直觉下"先不配"；出问题在峰值（批量导入、报表导出）。

**How to avoid:**
- 配置显式化：MySQL `MaxOpenConns=10~20 / MaxIdleConns=5 / ConnMaxLifetime=30min`，Redis `PoolSize=10`，全部进 YAML 可调；
- ConnMaxLifetime 必设（防 MySQL wait_timeout 杀连接后拿到坏连接）；
- 压测场景固定：50 并发混合读写 + 一次 1000 行 Excel 导入，观察连接数峰值。

**Warning signs:**
日志出现 `bad connection` / `too many connections`；请求随机超时但重启恢复。

**Phase to address:**
工程骨架阶段（配置模板默认值即正确值）。

---

### Pitfall 15: 分页与排序稳定性（PageHelper 语义 + 翻页漂移）

**What goes wrong:**
源 `PageHelper.parsePage`：page 缺省 1、size 缺省 15、均钳位 ≥1（`kernel/.../persistence/PageHelper.java`）；列表统一 `orderByDesc(createTime)`。问题：
- `create_time` 是秒级 DATETIME，同一秒插入多条时排序不稳定 → 翻页出现重复行/丢行（LIMIT/OFFSET 无稳定 tiebreaker）；
- Go 端若忘做钳位，`page=0` 或负数直接生成 `OFFSET -15` SQL 错误（Java 是自动纠偏，客户端可能依赖这个宽容度）；
- count 查询与记录查询非同一条件集时 total 错误（GORM 手写 count 容易漏 like 条件）。

**Why it happens:**
分页是"最简单"的功能，测试只测正常路径。

**How to avoid:**
- 封装统一分页函数：钳位语义（缺省 1/15）+ `Count()` 与 `Offset/Limit` 共用同一 `Where` 链；
- 排序加稳定 tiebreaker：`ORDER BY create_time DESC, id DESC`（行为增强但必要，API 重设计允许）；
- 契约测试：page=0/size=-1/超界页码 返回空列表不报错。

**Warning signs:**
前端翻页偶发重复数据；`page=0` 请求 500。

**Phase to address:**
数据访问层阶段（分页 helper 与自动填充同期实现）。

---

### Pitfall 16: 长运行进程的 goroutine 纪律（泄漏/panic/优雅退出）

**What goes wrong:**
Java 用线程池 + 容器托管生命周期；Go 版常驻单进程（systemd），典型事故：
1. 中间件/事件处理里 `go func()` 无 recover → 单个 nil map 写入 panic 整个进程退出，IAM 服务全断；
2. Excel 导入、文件流、excelize `Rows` 不 Close → fd/内存缓慢增长，工控机跑几周 OOM；
3. systemd 发 SIGTERM 直接 exit → Redis 会话正常但写一半的操作日志、Excel 导入事务回滚不干净；
4. Redis pub/sub（源有 RedisPubSubService）若移植，订阅循环错误退出后无人重连，静默失联。

**Why it happens:**
Go 的并发便宜到随手开 goroutine，但生命周期要手工管理。

**How to avoid:**
- 全局纪律：任何 `go func` 必须 (a) 顶层 recover (b) 持有可取消 context (c) 归属一个 waitgroup；
- 主流程显式生命周期：`signal.NotifyContext(SIGTERM)` → http.Server.Shutdown(超时 30s) → flush 日志队列 → 关闭 DB/Redis；
- 资源全部 `defer Close`（excelize/文件/S3 body）；
- 定时任务/订阅循环带 `context` 取消 + 指数退避重连；
- 验收：压测后 `runtime.NumGoroutine()` 与 fd 数回落基线；`kill -TERM` 后日志表无缺口。

**Warning signs:**
RSS 只涨不跌；重启后日志表缺最后几秒；`dmesg` 出现 OOM killer。

**Phase to address:**
工程骨架阶段（生命周期/优雅退出模板）；每个引入后台 goroutine 的阶段复检。

---

### Pitfall 17: 枚举三表示面（DB int / REST name / Excel 中文）映射丢失

**What goes wrong:**
同一枚举（如 `UsageStatus`）有三个表示面：DB 存 int value（`@EnumValue`：DISABLED=0/ENABLED=1）、REST 输出 name（"ENABLED"）、Excel 用中文 info（"正常"/"禁用"）；且**未知值兜底默认**（fromValue→ENABLED，fromInfo→ENABLED/UNKNOWN）。Go 若只做 int↔string 双向，Excel 中文面和兜底语义丢失；若 GORM 直接用自定义类型忘实现 `driver.Valuer`/`sql.Scanner`，落库变成 name 字符串，类型列是 int → 读取报错或静默写坏。

**Why it happens:**
Java 侧三个转换器分散在注解里（@EnumValue/Jackson Module/Excel Converter），移植者只看见其一。

**How to avoid:**
- Go 定义单一枚举类型，集中实现四件套：`MarshalJSON`（name）/`UnmarshalJSON`（name|int，未知兜底）/`Value()`（int）/`Scan()`（int）；
- Excel 转换独立函数（中文 info ↔ 枚举，未知兜底）；
- 锁定测试：`{"status":0}`、`{"status":"DISABLED"}`、`{"status":99}` 三种输入的行为全部断言。

**Warning signs:**
DB 里 status 列出现字符串；status=99 请求 500 而非默认值。

**Phase to address:**
数据访问层阶段（与 Pitfall 5 同期，属于共享 kernel 等价层）。

---

### Pitfall 18: 乐观锁/逻辑删除的"幽灵功能"误移植

**What goes wrong:**
`MybatisPlusConfig` 注册了 `OptimisticLockerInnerInterceptor`，但全库**没有任何 `@Version` 字段**（grep 证实）——乐观锁处于休眠状态；逻辑删除同样不存在（见 Pitfall 4）。移植者若"补全"这些能力（给模型加 version 字段 + GORM Optimistic lock 实现），语义就变了：更新可能因 version 不匹配而静默失败（影响行数 0），与源"后写胜"行为冲突。

**Why it happens:**
配置文件里有插件名，望文生义以为在用。

**How to avoid:**
- 明文记录：乐观锁=未启用，不移植；并发覆盖语义与源一致（last-write-wins）；
- 移植清单以"实际生效的行为"为准而非"配置了什么"。

**Warning signs:**
code review 出现 version 字段；更新偶发 no-rows-affected。

**Phase to address:**
数据访问层阶段（在模型约定中注明）。

---

## Technical Debt Patterns

| Shortcut | Immediate Benefit | Long-term Cost | When Acceptable |
|----------|-------------------|----------------|-----------------|
| 更新直接用 `Save()` 不做指针 DTO | 移植速度快一倍 | 生产数据被零值静默清空，最难追查的一类事故 | never |
| ID 用 JSON 数字输出 | 少写一个自定义类型 | 前端精度丢失，更新/删除错位 | never（雪花 ID 场景） |
| 时间用 `time.Time` 直出 RFC3339 | 少写 MarshalJSON | 前端解析器/测试/导出格式三处适配 | 若 API 全面重设计且前端同步改，可接受——但要显式决策而非默认 |
| 操作日志同步写库 | 实现简单、零丢失 | 峰值时请求延迟加一次写库耗时 | 工控机低并发下可接受（源即如此），量上来再异步化 |
| Excel 按列序映射 | 快 | 模板一改全错位 | never |
| 事件改 goroutine 异步 | "性能更好" | 竞态测试失败、退出丢事件、顺序不可控 | never（单机场景同步足够） |

## Integration Gotchas

| Integration | Common Mistake | Correct Approach |
|-------------|----------------|------------------|
| MySQL DSN | 缺 `parseTime=True`（time.Time 扫描报错）、`loc` 缺省 UTC（差 8h）、`charset` 缺省非 utf8mb4（emoji/生僻字写入失败） | `charset=utf8mb4&parseTime=True&loc=Asia%2FShanghai` 固化进配置模板 |
| Redis | 复用源 `satoken:` key 前缀造成新旧格式混存误读 | 换新前缀（如 `sysiam:`）；上线 runbook 清空旧会话键（停机切换已定） |
| RustFS(S3) | 下载 SDK 对象后 `ReadAll` 进内存再回写 | GetObject 返回的 Reader 直接 `io.Copy` 到响应流，errdefer Close |
| systemd | 无 SIGTERM 处理，10s 后被 SIGKILL 硬杀 | unit 配 `TimeoutStopSec=30` + 应用优雅退出链（Pitfall 16） |
| Nacos 配置迁移 | 拍脑袋填 sa-token timeout/JWT 密钥等不可见配置 | 向用户索取 Nacos craft-satoken.yaml / craft-shared.yaml 实际值（MEDIUM：本仓库不可见） |

## Performance Traps

| Trap | Symptoms | Prevention | When It Breaks |
|------|----------|------------|----------------|
| 文件下载 byte[] 全量载入（源行为） | RSS 阶梯式上涨，OOM | 流式 `io.Copy`（Pitfall 13） | 单文件 > 数百 MB 或并发 2-3 个下载 |
| Excel 导入逐行查重（源行为，N+1） | 1000 行导入几十秒 | 单机本地 MySQL 可接受；量大改批量 IN 预查 | 行数 > 数千 |
| 连接池无上限 | 峰值连接耗尽全员超时 | 显式池配置（Pitfall 14） | 并发 > ~50 或批量导入并发业务 |
| `list()` 全表加载建树（Dept/Menu 源行为） | 每请求全表扫描 | 菜单/部门量级 < 千行，保持全量加载是对的——不要过度优化成递归 SQL | 仅当行数上万 |
| 操作日志 params 无上限截断 | 超大请求体打进 TEXT 列 | 源已限 3 参数 + errorMsg 500 字符；params 本身再加长度上限 | 大 payload POST |

## Security Mistakes

| Mistake | Risk | Prevention |
|---------|------|------------|
| 错误响应回显内部详情（SQL/类名/路径） | 信息泄漏 | 源兜底分支固定文案（GlobalExceptionHandler.java:85-90），Go 版保持：细节只进服务端日志 |
| 操作日志 params 未掩码密码/token | 敏感信息进日志表 | 原样移植敏感正则掩码（password/secretKey/apiKey/token 等 7 字段→`******`） |
| 区分"用户不存在"与"密码错误" | 用户名枚举 | 统一"用户名或密码错误"（源行为） |
| API Key 明文落日志/响应 | 凭据泄漏 | X-API-Key 头不进日志 params；ApiKey 实体输出脱敏 |
| 权限会话永不失效（timeout 误配 -1） | 离职账号长期残留会话 | timeout 配置取 Nacos 实际值并写入部署文档 |

## UX Pitfalls

| Pitfall | User Impact | Better Approach |
|---------|-------------|-----------------|
| 改角色需重新登录才生效（源语义保留） | 用户困惑权限没变 | 保持行为 + 前端提示"重新登录后生效"；不要擅自改成实时生效 |
| 重置密码固定为 123456（源行为） | 安全弱 | 保持（运营依赖默认密码流程），但保留 firstLogin=true 强制改密语义 |
| 分页 size 无上限 | size=100000 一次拉全表 | 钳位 size 上限（如 100），API 重设计范围内 |
| 时间显示带时区后缀 | 前端二次转换出错 | 输出无时区 "yyyy-MM-dd HH:mm:ss" 字符串 |

## "Looks Done But Isn't" Checklist

- [ ] **认证：** 存量 BCrypt hash 可登录（拿生产真实 hash 验证，不是新建用户自测）
- [ ] **认证：** API Key 登录复用已有会话分支（`loginByApiKey` 的双路径）
- [ ] **用户编辑：** 不传的字段不被清空（指针 DTO 全覆盖验证）
- [ ] **内置用户：** builtin=true 删除被硬拦截且零删除调用（源在服务层前置判断）
- [ ] **部门/字典/菜单改名：** 6 个冗余字段全部同步（user.deptName / dept.parentName / dictData.dictTypeName / role-menu 相关字段逐一核对）
- [ ] **操作日志：** 失败请求也记录（status=0）且掩码生效
- [ ] **分页：** page=0、size=-1、超界页码三个边界不 500
- [ ] **Excel 导入：** 用源系统真实导出文件回测（不是程序生成的完美文件）
- [ ] **文件下载：** Content-Disposition 中文文件名编码（源 URLEncoder + `%20` 替换）
- [ ] **健康检查：** /actuator/health 等价端点 + Prometheus 指标（PROJECT.md 要求）
- [ ] **枚举兜底：** status=99、性别乱码等未知值不 500，走默认值
- [ ] **优雅退出：** SIGTERM 后进行中的请求与操作日志完整落库

## Recovery Strategies

| Pitfall | Recovery Cost | Recovery Steps |
|---------|---------------|----------------|
| 零值更新清了数据（Pitfall 3） | HIGH | 从 MySQL binlog/备份恢复受影响行；无法恢复时人工补录；立即给更新 helper 加 Select 白名单 |
| ID 精度丢失已上线（Pitfall 2） | MEDIUM | 后端改为字符串输出（纯增量变更），前端刷新即愈；期间错写的关联数据需人工核对 |
| 时区差 8 小时已入库（Pitfall 6） | HIGH | 定位漂移窗口，批量 ±8h 修正受影响行；修正 DSN 后再验证 |
| 畸形 hash 登录 500（Pitfall 7） | LOW | 封装 error→false 即修复；受影响用户走重置密码（123456）流程 |
| goroutine 泄漏数周后 OOM（Pitfall 16） | MEDIUM | systemd 自动重启止血；用 pprof 定位泄漏点修复；补 fd/goroutine 监控指标 |
| Excel 列错位批量写坏（Pitfall 12） | HIGH | 事务整批可回滚重导（源即整批事务）；非事务导入需按导入时间窗回删 |
| Redis 会话键混存（Pitfall 10） | LOW | FLUSHDB 会话库，全员重新登录（停机切换场景本来就要做） |

## Pitfall-to-Phase Mapping

> 阶段名为按依赖关系的逻辑称呼，roadmap 定稿后替换为实际编号。

| Pitfall | Prevention Phase | Verification |
|---------|------------------|--------------|
| 1 雪花 ID 生成 | 数据访问层阶段 | 插入测试断言 ID 非零、无 DB 自增假设 |
| 2 ID JSON 字符串化 | API 骨架阶段 | 序列化契约测试：id 输出为字符串 |
| 3 零值更新 | 数据访问层阶段（helper）+ 各功能域 | 编辑不传字段保持原值的集成测试 |
| 4 禁用软删除 | 数据访问层阶段 | code review checklist + 全模型 grep |
| 5 JSON 契约三特征 | API 骨架阶段 | 翻译源 JacksonConfigTest 为 Go 契约测试 |
| 6 时区 | 工程骨架阶段（DSN）+ API 骨架（序列化） | TZ=UTC 环境跑集成测试无漂移 |
| 7 BCrypt 互操作 | 认证阶段 | 生产 hash 抽样验证脚本通过 |
| 8 审计自动填充 + 全表防护 | 数据访问层阶段 | 列表页创建人非空；grep 无裸全局 Update |
| 9 事件同步语义 | 事件/名称同步阶段 | 改名后冗余字段同步的集成测试（含 NULL 行为） |
| 10 Redis 会话语义 | 认证阶段（配置值在工程骨架确认） | 权限快照不实时生效断言；续期 TTL 重置断言 |
| 11 操作日志切面 | 操作日志阶段 | 失败请求有日志、掩码生效、body 回放后接口正常 |
| 12 Excel 导入 | 导入功能域阶段 | 源系统真实文件 fixtures 回归 |
| 13 文件流式化 | 文件功能域阶段 | 大文件下载 RSS 平稳压测 |
| 14 连接池 | 工程骨架阶段 | 50 并发压测连接数受控 |
| 15 分页语义 | 数据访问层阶段 | 边界参数测试 + 翻页无重复行 |
| 16 goroutine 纪律 | 工程骨架阶段（模板）+ 各阶段复检 | SIGTERM 演练 + 长跑 fd/goroutine 基线 |
| 17 枚举三面 | 数据访问层阶段 | 三种输入形态的枚举测试矩阵 |
| 18 不移植休眠功能 | 数据访问层阶段 | 模型无 version/deletedAt 字段 |

## Sources

- 源码审读（HIGH，全部结论的一手证据）：`sys-kernel/.../config/{AutoFillHandler,MybatisPlusConfig,JacksonConfig,EnumByValueJacksonModule,SaTokenConfig,GlobalExceptionHandler,UserContextCleanupFilter}.java`、`kernel/.../persistence/{PageHelper,KtQuery}.java`、`kernel/.../domain/entity/{BaseAggregate,BaseUserEntity}.java`、`sys-iam/.../persistence/*RepositoryImpl.java`、`application/service/{UserApplicationService,AuthApplicationService,FaultInfoApplicationService}.java`、`application/listener/{OperationLogAspect,DeptNameSyncListener}.java`、`common-utils/.../excel/ExcelHelper.java`、`starter-redis/RedisJsonSupport.java`、`sys-iam/build.gradle.kts`（sa-token-redis-jackson 依赖确认）
- GORM 官方文档（context7，HIGH）：零值更新与 Select/Omit（gorm.io/docs/update.html）、MySQL DSN 参数与 autoCreateTime（gorm.io/docs/connecting_to_the_database.html, models.html）
- Excelize 官方文档（context7，HIGH）：Rows 流式迭代、v2.5.0 Close 强制（xuri/excelize-doc）
- bcrypt 互操作性（web，HIGH）：[Wikipedia Bcrypt（$2a/$2b/$2y 前缀语义）](https://en.wikipedia.org/wiki/Bcrypt)、[在线 $2a/$2b/$2y 验证器](https://beavrxyz.netlify.app/tools/developer/bcrypt)
- 雪花 ID JS 精度（web，HIGH）：CSDN《Mybatis-Plus 主键 ASSIGN_ID 雪花 id 长度以及 JS 精度问题解决》等（Long→String 方案与源项目 JacksonConfig 互相印证）
- sa-token-redis-jackson key 结构（web，MEDIUM）：[SaTokenDaoRedisJackson 源码](https://github.com/UyCode/sa-token/blob/dev/sa-token-dao-redis-jackson/src/main/java/cn/dev33/satoken/dao/SaTokenDaoRedisJackson.java)、Sa-Token 发行版 changelog（Jackson 定制 Session）
- 信息缺口（需向用户确认）：Nacos 中 `craft-satoken.yaml` 的 timeout/active-timeout 实际值、`craft-shared.yaml` 的 datasource/redis 连接参数、MySQL 库的 collation（utf8mb4_general_ci 还是 unicode_ci）

---
*Pitfalls research for: sys-iam-go（Java→Go IAM 后端移植）*
*Researched: 2026-10-31*
