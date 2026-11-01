---
phase: 04-domains
plan: 02
subsystem: api
tags: [file-storage, minio, s3, rustfs, multipart, md5-dedup, streaming, filecategory, gin, gorm]

# Dependency graph
requires:
  - phase: 04-domains
    plan: 01
    provides: sys_file/sys_file_category DDL 复刻（testutil glob 自动加载）+ file 域 7 串 perm 常量 + 轻 CRUD 四层模板与 RequirePerm 路由收口纪律
  - phase: 02-rbac
    provides: newTestApp 双容器集成测试基建 + parseID/薄层 handler 样板 + dept 树域模板（internal/tree 孤儿提升）
provides:
  - FileStorage 抽象（Put/GetStream/Delete 三方法接口 + storage.New 按 driver 分派，local 延迟形态不触碰 minio）
  - LocalStorage（resolveKey 路径逃逸双防御）/S3Storage（minio-go v7.4.1，EnsureBucket fail-fast，StatObject 前置取 size）双实现
  - StorageConfig 配置段（driver/local.root/s3 五参数 + envKeyAliases 7 键登记 + fail-fast 校验，凭证不入错误消息）
  - File 域全链：上传 MD5 判重五语义（categoryId eq/IS NULL 双分支，拒绝零副作用）+ 流式下载（Content-Length + RFC 5987 attachment 头）+ 宽容删除（storage 失败仅 slog.Warn 继续删 DB）
  - FileCategory 域全链：CRUD + 树（孤儿提升）+ parent_name 冗余 + WR-04 成环拦截
  - BodyLimitWithMB(mb) 路由级独立上限 + 全局 BodyLimit 对 multipart 跳过（IN-10 预留设计落地）+ MaxMultipartMemory 8MB
  - 9 条新路由逐条 RequirePerm（router.go 收口 66；download 挂 PermFileDownload——D-16 核对菜单存在非 super_admin 专属）
affects: [04-03-olog, 04-04-fault]

# Actuals (#2632) — pairs with the plan's `estimate` to calibrate future estimates.
actuals:
  tokens: 24000    # chars/4 over the realized diff (24 files, 2334 insertions / 38 deletions)
  tasks: 2
  commits: 3
commits: 3
plan_head_before: 3df4ef5cc73e8a62889979ba4dc4f9ba4279f7ed
plan_head_after: 6748849c5363590153e8a3ad9bce29b070c27df4

# Tech tracking
tech-stack:
  added:
    - github.com/minio/minio-go/v7 v7.4.1（STACK 锁定版本，纯 Go 无 CGO——Task 1 引入）
  patterns:
    - "Seek(0) 重放上传流模式：multipart.File 先 io.Copy(md5) 判重、拒绝零副作用、命中前不 Put，替代研究期 TeeReader 表述（D-12 语义不变）"
    - "multipart 双层封顶模式：全局 BodyLimit 按 Content-Type 前缀跳过 multipart + 路由级 BodyLimitWithMB 独立上限（外层先包则内层恒失效）"
    - "流式出站模式（仓库首例）：storage.GetStream → c.DataFromReader(size) 恒不整载；S3 侧 StatObject 前置把 404 挡在响应头写出前"
    - "存储侧宽容幂等对齐：LocalStorage os.IsNotExist 宽容 ≡ S3 RemoveObject 幂等，双实现同一 Delete 语义"

key-files:
  created:
    - internal/storage/storage.go
    - internal/storage/local.go
    - internal/storage/s3.go
    - internal/storage/local_test.go
    - internal/storage/s3_test.go
    - internal/model/po/file.go
    - internal/model/dto/file.go
    - internal/repository/file_info.go
    - internal/repository/file_category.go
    - internal/service/file.go
    - internal/service/file_category.go
    - internal/handler/file.go
    - internal/handler/file_category.go
    - internal/handler/file_integration_test.go
  modified:
    - internal/config/config.go
    - internal/config/config_test.go
    - configs/config.yaml
    - internal/middleware/body_limit.go
    - internal/app/app.go
    - internal/app/router.go
    - internal/handler/user_integration_test.go
    - internal/app/shutdown_integration_test.go

key-decisions:
  - "判重五语义测试蓝本直译：源 FileUploadHashDedupTest ①同分类同内容 400 源消息+零副作用（DB 0 新行+存储 1 对象）②异分类放行 ③IS NULL 分支命中 ④异内容放行 ⑤分类不存在 404 源消息"
  - "上传链顺序照源（save DB→上传存储且事务含存储调用）：tx 内 Create→storage.Put，Put 失败事务回滚 sys_file 零残留；Seek(0) 重放替代 TeeReader（拒绝零副作用语义不变）"
  - "上级分类不存在译为 404/10300（CodeNotFound）源消息原文——与部门域 400 不同，04-02 PLAN Task 2 action 5 显式拍板"
  - "FileCategory Update 加 WR-04 成环拦截（自引用/挂自身子孙拒绝）——plan 未显式列出但属「照 dept.go 模板」同构面，与菜单/部门域一致防树查询静默丢节点"
  - "Content-Disposition 手写 RFC 5987（filename*=UTF-8'' + url.PathEscape）——中文/特殊字符安全承载且天然消除 CR/LF 头注入面（T-4-03），恒 attachment（T-4-05 缓解面）"
  - "newTestApp 签名不变：Storage 段 local + t.TempDir() 内部注入，落盘根经 testStorageRoot 包级变量供判重零副作用断言直查"

patterns-established:
  - "File 域上传链七步模板：fh.Open→分类校验→流式 MD5→判重→Seek(0)→uuid_key→事务内 Create+Put（04-04 fault import 复用 multipart 处理面）"
  - "大 payload 路由注册模板：pri.POST(..., middleware.BodyLimitWithMB(N), RequirePerm(...), handler)——04-04 fault import 直接套用"

requirements-completed: [FILE-01, FILE-02, FILE-03, FILE-04, FILE-05]

coverage:
  - id: D1
    description: 上传 MD5 判重五语义：同分类同内容 400 源消息「文件已存在：{名}（内容 MD5 重复，同一分类下不可重复上传）」+ 零副作用（sys_file 0 新行+存储 1 对象）/异分类放行/未分类 IS NULL 分支命中/异内容放行/分类不存在 404 源消息；category_name 服务端冗余
    requirement: FILE-01
    verification:
      - kind: integration
        ref: internal/handler/file_integration_test.go#TestFileUploadDedupFiveSemantics
        status: pass
    human_judgment: false
  - id: D2
    description: 流式下载：2MB 文件 download 200 + Content-Length == size + 响应体逐字节一致（GetStream→DataFromReader 全链不整载）+ 不存在 id 404 域消息 + 分页列表 envelope
    requirement: FILE-02
    verification:
      - kind: integration
        ref: internal/handler/file_integration_test.go#TestFileDownloadStream
        status: pass
    human_judgment: false
  - id: D3
    description: 宽容删除：上传→删除 200 且 DB 行消失+存储对象移除；不存在 id 404（源 slog.Warn 宽容语义由代码路径保证，LocalStorage 成功路径覆盖主链）
    requirement: FILE-03
    verification:
      - kind: integration
        ref: internal/handler/file_integration_test.go#TestFileDeleteTolerant
        status: pass
    human_judgment: false
  - id: D4
    description: 文件分类 CRUD+树：两层级嵌套+parent_name 冗余（DB 行+树节点双断言）/孤儿 SQL 直插提升为根/上级分类不存在 404/10300 源消息/叶子 children []非 null/显式移至根 parent_name 清空
    requirement: FILE-04
    verification:
      - kind: integration
        ref: internal/handler/file_integration_test.go#TestFileCategoryTree
        status: pass
    human_judgment: false
  - id: D5
    description: storage.driver 配置切换：local 分支零 minio 触碰（延迟形态）；driver=s3 构造内 EnsureBucket fail-fast；rustfs 真容器 Put/Get/Delete 往返 smoke 绿（不可达 skip 策略）；同一 FileService 代码路径对双后端无差别
    requirement: FILE-05
    verification:
      - kind: integration
        ref: internal/storage/s3_test.go#TestS3StorageSmoke
        status: pass
      - kind: unit
        ref: internal/storage/local_test.go#TestLocalStorage*
        status: pass
    human_judgment: false
  - id: D6
    description: 工控机 DoS 防护面：全局 BodyLimit 1MB 对 multipart 跳过 + upload 路由 BodyLimitWithMB(20) 唯一封顶 + MaxMultipartMemory 8MB 落盘阈值；9 路由逐条 RequirePerm 收口 66（57+9），download 挂 PermFileDownload（D-16）
    requirement: FILE-01
    verification:
      - kind: other
        ref: "grep -c middleware.RequirePerm internal/app/router.go == 66; grep -c BodyLimitWithMB( == 1"
        status: pass
      - kind: integration
        ref: internal/handler/file_integration_test.go#TestFileUploadDedupFiveSemantics
        status: pass
    human_judgment: false

# Metrics
duration: 75min（含中断恢复：Task 1 会话 + Task 2 恢复会话）
completed: 2026-11-01
status: complete
---

# Phase 4 Plan 02: FileStorage 抽象（本地/S3 切换）+ File 域（上传判重/流式下载/宽容删除）+ FileCategory Summary

**FileStorage 三方法接口 local/s3 双实现（minio-go v7.4.1）+ File 域五语义 MD5 判重（categoryId IS NULL 双分支、拒绝零副作用、Seek(0) 重放）+ 流式下载 + 宽容删除 + FileCategory 树——四个集成测试全绿且全量回归 18 包 ok**

## Performance

- **Duration:** 75 min（Task 1 会话约 30 min + 中断恢复后 Task 2 会话约 45 min）
- **Started:** 2026-11-01T09:55:00Z（Task 1）/ 2026-11-01T10:16:00Z（Task 2 恢复）
- **Completed:** 2026-11-01T11:06:00Z
- **Tasks:** 2
- **Files modified:** 24（14 新建 + 10 修改）

## Accomplishments
- FileStorage 抽象落地：Put/GetStream/Delete 三方法接口，storage.New 按 driver 分派——local 分支构造链零 minio 符号触碰（D-10 延迟形态），s3 构造内 EnsureBucket fail-fast（等价源 @PostConstruct createBucket）
- LocalStorage resolveKey 双防御（Clean 吃穿越片段 + 落点前缀校验，local_test 直测锁定，T-4-02）；S3Storage StatObject 前置（minio GetObject 错误延迟浮现坑——404 在响应头写出前暴露）+ PutObject 真实 size 直传（禁 -1 分片内存缓冲 prohibition）
- StorageConfig 段：driver/local.root/s3 五参数，envKeyAliases 7 键全登记（WR-01 多段 snake_case 静默丢失面清零），fail-fast 校验错误消息不含凭证值（T-4-04）
- File 域上传链七步（fh.Open→分类校验→流式 MD5→判重→Seek(0)→uuid_key→事务内 Create+Put）：判重命中前零存储/DB 副作用（集成断言 sys_file 行数+存储对象数双证），Put 失败事务回滚（源 save DB→存储同序横切发现 6）
- FileCategory 与 dept 全同构：父校验 + parent_name 服务端冗余 + tree.Build 孤儿提升 + WR-04 成环拦截；上级分类不存在按 plan 拍板 404/10300 源消息
- BodyLimit 参数化：BodyLimitWithMB(20) 路由级独立上限 + 全局 1MB 对 multipart 跳过（IN-10 预留设计落地——外层先包 1MB 会使内层恒失效的中间件次序坑注释化）；MaxMultipartMemory 8MB 落盘阈值
- 9 条路由逐条 RequirePerm（收口 66=57+9）：download 挂 PermFileDownload（D-16 核对菜单存在非 super_admin 专属）；9 条含 tree 即列表档共用 PermFileList（源 fileCategory 与 file 共用 file:* 权限）

## Task Commits

Each task was committed atomically:

1. **Task 1: FileStorage 接口 + Local/S3 双实现 + storage 配置段 + 存储层测试** - `a93a31e` (feat)
2. **Task 2: File 域全链（判重五语义/流式下载/宽容删除）+ FileCategory 树 + 路由挂载** - `6748849` (feat)

**Plan metadata:** 见最终 docs commit（本文件 + STATE/ROADMAP）

## Files Created/Modified
- `internal/storage/storage.go` - FileStorage 三方法接口 + New 按 driver 分派（包文档头记录延迟形态与双实现语义对齐点）
- `internal/storage/local.go` - LocalStorage：resolveKey 逃逸双防御 + Put 按需 MkdirAll + GetStream Stat 取 size + Delete IsNotExist 宽容
- `internal/storage/s3.go` - S3Storage：endpoint scheme 规范化 + EnsureBucket fail-fast + PutObject 真实 size + StatObject 前置 GetStream
- `internal/storage/local_test.go` / `s3_test.go` - LocalStorage 四单测（免容器）+ S3 smoke（rustfs 真容器，env 覆盖 + 2s TCP 探测不可达 skip）
- `internal/config/config.go` + `config_test.go` + `configs/config.yaml` - StorageConfig 段 + envKeyAliases 7 键 + defaults/fail-fast + yaml storage 段注释
- `internal/middleware/body_limit.go` - BodyLimit multipart 跳过分支 + BodyLimitWithMB(mb) 路由级上限
- `internal/model/po/file.go` - FileInfoPO（sys_file 16 列）+ FileCategoryPO（sys_file_category 15 列），BeforeCreate 雪花/审计同构
- `internal/model/dto/file.go` - FileCategory Create/Update/Resp/TreeResp（dept 同构）+ FileInfoResp + 手写 ToXxxResp
- `internal/repository/file_info.go` - CRUD + FindByCategoryAndHash（eq/IS NULL 双分支 + LIMIT 1）+ 分页闭包 List（categoryId 可选过滤，create_time DESC, id DESC）
- `internal/repository/file_category.go` - CRUD + 全量 List（sort ASC, id ASC）
- `internal/service/file.go` - Upload 七步链 + Download 载体 + Delete 宽容链（slog.Warn 源日志原文）+ List 分页 envelope
- `internal/service/file_category.go` - Create/Update 父校验冗余 + Get/List/Tree + ensureNotDescendant 成环拦截
- `internal/handler/file.go` - Upload（FormFile+PostForm）+ Download（RFC 5987 attachment 头 + DataFromReader 流式）+ List（paging.Parse + category_id）+ Delete
- `internal/handler/file_category.go` - 五方法照 dept handler
- `internal/app/app.go` - storage.New fail-fast 装配（service 构造前）+ File/FileCategory repo→svc→handler 链 + handlers 两字段
- `internal/app/router.go` - MaxMultipartMemory 8MB + 9 路由逐条 RequirePerm（upload 叠加 BodyLimitWithMB(20)）
- `internal/handler/user_integration_test.go` - newTestApp cfg 增 Storage 段（t.TempDir()）+ testStorageRoot 包级变量（签名不变）
- `internal/handler/file_integration_test.go` - 四集成测试（判重五语义/下载流式/宽容删除/分类树）
- `internal/app/shutdown_integration_test.go` - cfg 增 Storage 段（Rule 3 回归修复）

## Decisions Made
- 判重命中消息带**本次上传原名**（f2.txt 非 f1.txt）——源 FileApplicationService:79-80 语义，集成测试逐字断言
- Download 复用 errcode 分段契约：文件不存在 404/10300、文件路径为空 400/10400（源消息原文两条）
- 未上传流整载面零容忍：io.Copy(md5.New()) 判重、DataFromReader(io.Copy) 出站，代码路径无 ReadAll（FILE-02 RSS 验收由 2MB 下载测试 + 代码审查双证）
- file list 分页 1/15 标准钳位（横切发现 4：file 是分页域非全量域）；category_id 过滤在 handler 解析非法 400
- 上传 key 形态 uuid+"_"+原名（源 filePath 形态对齐；uuid 前缀防同名互覆，LocalStorage resolveKey 对键做逃逸防御兜底）

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] shutdown_integration_test cfg 缺 Storage 段**
- **Found during:** Task 2 全量回归（internal/app FAIL）
- **Issue:** app.New 现在无条件构造 FileStorage，shutdown_integration_test 自建 config.Config 无 Storage 段 → `init file storage: storage: unknown driver ""` 装配失败（本 plan 改动直接波及）
- **Fix:** 该测试 cfg 增 Storage local + t.TempDir()（与 newTestApp 同构最小装配）
- **Files modified:** internal/app/shutdown_integration_test.go
- **Verification:** TZ=UTC go test -tags=integration ./internal/app/ 全绿；全量回归 18 包 ok
- **Committed in:** 6748849（Task 2 commit 内）

**2. [Rule 1 - Bug] router.go 注释字面量污染 grep gate 计数**
- **Found during:** Task 2 verify gate 自检
- **Issue:** 两处注释含 `BodyLimitWithMB(20)` 文本使 grep -c 计 3（gate 要求恰 1 处路由挂载）
- **Fix:** 注释措辞改为「20MB 路由级独立上限」语义表述（实现代码不变），计数精确 1
- **Files modified:** internal/app/router.go
- **Verification:** grep -c "BodyLimitWithMB(" == 1；RequirePerm == 66
- **Committed in:** 6748849（Task 2 commit 内）

**3. [Rule 1 - Bug] file_integration_test 未使用变量编译失败**
- **Found during:** Task 2 集成测试首跑
- **Issue:** TestFileDownloadStream 解构 dsn 未使用（Go 编译错误 declared and not used）
- **Fix:** 解构改 `_` 占位
- **Files modified:** internal/handler/file_integration_test.go
- **Verification:** 四个 TestFile 用例全绿
- **Committed in:** 6748849（Task 2 commit 内）

---

**Total deviations:** 3 auto-fixed（1× Rule 3 阻断 + 2× Rule 1 bug——均为装配契约与 gate/编译层面修正，实现语义零偏差）
**Impact on plan:** 全部修正为本 plan 改动的直接波及面收敛，无 scope creep。

## Issues Encountered
- 恢复执行会话：Task 1 已于中断前完整提交（a93a31e），按 resume 交接从 Task 2 起步，git log/status 盘点后对照 PLAN 无重做
- 全量集成回归 850s（handler 包）转 background 完成——testcontainers 双容器 per-test 启动为既有基建形态
- S3 smoke 于 rustfs 真容器（localhost:9000）实测跑绿（非 skip 路径）——EnsureBucket 幂等建桶 + Put/Get/Delete 往返断言 0.01s

## User Setup Required
None - no external service configuration required（minio-go v7.4.1 经 go.mod 锁定拉取；现场 S3 凭证经 configs/config.yaml 或 IAM_STORAGE_S3_* 环境变量配置，默认值照源 RustfsProperties 标注「现场部署替换」）。

## Next Phase Readiness
- 04-03（Olog）路由挂载点齐备：RequirePerm 收口 66，Olog 增量路由按 oplog 域常量（log 域 1 串已在 perms.go）挂载即可
- 04-04（Fault）可直接复用大 payload 路由模板（BodyLimitWithMB + multipart 跳过已落地）与 multipart 处理面（fh.Open/表单字段模式）
- FileStorage 抽象稳定：fault import 不涉存储；File 域删除宽容语义已双实现对齐，后续无需改动 storage 包
- newTestApp 已含 Storage 段——后续 plan 集成测试零装配改动（Phase 1-3 既有测试不受影响已证）

---
*Phase: 04-domains*
*Completed: 2026-11-01*

## Self-Check: PASSED

- 关键文件存在性验证：internal/storage 5 文件 + File 域 8 新文件 + 3 测试文件全 FOUND（下述命令实测）
- 2 个任务提交（a93a31e/6748849）均为 HEAD 祖先（merge-base --is-ancestor 实测）
- 无意外文件删除（2 task commits diff-filter=D 为空）；无未跟踪代码文件遗留（.planning 编排器文件除外）
- 结构 gate 实测：RequirePerm 66 / BodyLimitWithMB 1 / DDL 17 文件（三推导式全成立）
- 集成验证：TestFile 四用例全绿；S3 smoke rustfs 真容器绿；全量回归 18 包 ok（internal/app 修复后）
