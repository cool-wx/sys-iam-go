---
phase: "04"
slug: "04-domains"
status: verified
# threats_open = count of OPEN threats at or above workflow.security_block_on severity (the blocking gate)
threats_open: 0
asvs_level: 1
created: "2026-11-01"
---

# Phase 04 — Security

> Per-phase security contract: threat register, accepted risks, and audit trail.

---

## Trust Boundaries

| Boundary | Description | Data Crossing |
|----------|-------------|---------------|
| 客户端→六域 CRUD 端点（04-01） | 已认证但未授权的请求经 RequirePerm gate 进入 Dict/Config/Shift 数据面（沿用 Phase 3 边界，无新增匿名面） | 业务 JSON（dict/config/shift 字段） |
| 客户端→upload/download 端点（04-02） | 已认证已授权请求提交任意字节流（multipart 文件 + 表单字段），内容完全不受信 | 任意文件字节流 + 元数据（fileName/contentType） |
| 服务→存储后端（S3/RustFS，04-02） | 凭证经配置注入出站连接；key/filePath 落库后回用于存储寻址 | S3 凭证（secretKey）+ 对象字节流 |
| 存储元数据→HTTP 响应头（04-02） | fileName/contentType 取自 DB（源于上传原名），回写 Content-Disposition/Content-Type | HTTP 响应头（头注入面） |
| 请求体→日志存储（04-03） | 任意客户端可控 JSON 进入 params 缓存并最终落 sys_operation_log（跨信任域持久化） | 请求体 JSON（含敏感字段形态）+ 内部错误消息 |
| 查询参数→SQL（04-03） | module/operation/real_name 用户可控字符串进入 LIKE 查询 | 日志查询过滤串 |
| 客户端→import 端点（04-04） | 已授权用户提交任意 xlsx 字节流（行数/单元格内容/体积完全不受信） | xlsx 字节流（zip 结构） |
| Excel 单元格→DB 写路径（04-04） | 单元格字符串直接进入列值与三键查询参数（整批事务内） | 业务字符串 + SQL 参数 |

---

## Threat Register

| Threat ID | Category | Component | Severity | Disposition | Mitigation | Status |
|-----------|----------|-----------|----------|-------------|------------|--------|
| T-4-01 | Information Disclosure | 四域 list 端点（dict/config/shift 无行级过滤） | medium | accept | D-03 拍板本阶段零 dataScope 应用（需求未要求，通用件已备 v2 挂接）；功能权限面已由 RequirePerm 全量拦截（UAT 实测 401/403 面） | closed |
| T-4-02 | Tampering/Elevation | LocalStorage key→文件系统路径 | high | mitigate | resolveKey 双防御（filepath.Clean + 落点前缀校验，internal/storage/local.go 实证 2 处；local_test.go 四单测免容器直测）；下载寻址只用 DB 内 file_path 非客户端参数 | closed |
| T-4-03 | Tampering | 下载响应头（fileName 进 Content-Disposition） | medium | mitigate | 手写 filename*=UTF-8'' + url.PathEscape（handler/file.go 实证）；UAT 实测响应头 `Content-Disposition: attachment; filename*=UTF-8''f1.txt`——CR/LF 头注入面消除 | closed |
| T-4-04 | Information Disclosure | S3 凭证（secretKey） | high | mitigate | config fail-fast 错误消息不含凭证值；slog 全程不打 storage 凭证字段；config.yaml 默认值注释标注「现场部署替换」（04-02 SUMMARY + config_test.go 验证） | closed |
| T-4-05 | Repudiation/Tampering | 上传文件内容不扫描（恶意载荷入库入存储） | medium | accept | 源系统同语义；缓解面：上传/下载全经 RequirePerm 鉴权 + 下载恒 attachment 头（UAT 实测）不以同源 inline 渲染 + 单机内网部署 | closed |
| T-4-06 | DoS | upload 端点大 payload（multipart 内存放大） | high | mitigate | BodyLimitWithMB(20) 路由级上限（router.go:174 实证）+ 全局 1MB 对 multipart 跳过后仍有路由级封顶 + MaxMultipartMemory 8MB 落盘阈值（router.go:54 实证） | closed |
| T-4-07 | Information Disclosure | Oplog params 列（敏感凭据明文落库） | high | mitigate | 落库前 8 字段正则掩码（sensitiveRe 单正则 8 字段实证）+ 掩码先于 60KB 截断顺序锁定 + 逐字段单测；UAT 实测 login 日志 params password=「******」 | closed |
| T-4-08 | DoS | Oplog 异步链（chan 满/落库慢拖垮业务） | high | mitigate | Enqueue select-default 丢弃永不阻塞（op_log.go default 分支实证 + 单测直证）+ worker 落库错误仅 Warn 不反压 + chan 512 × 60KB ≈30MB 内存封顶 | closed |
| T-4-09 | DoS | 掩码正则（ReDoS 面） | low | mitigate | Go regexp 为 RE2 线性时间引擎（sensitiveRe 无嵌套量词实证）；结构性免疫 + 单测大输入锁定 | closed |
| T-4-10 | Information Disclosure | error_msg 列（内部错误细节） | medium | mitigate | error_msg 源头为 errcode.Error 已收敛域文案（UAT 实测 errorMsg=「字典类型已存在」非内部堆栈）+ 截 500 二次封顶 | closed |
| T-4-11 | DoS | import 端点（xlsx 解析内存/行数放大） | high | mitigate | BodyLimitWithMB(20) 体积上限（router.go:204 实证）+ maxImportRows=5000 流式读取阶段前置拒绝（fault_import.go:38 实证，WR-03）+ MaxMultipartMemory 8MB——三维封顶；UAT 实测表头/坏行防线在解析前生效 | closed |
| T-4-12 | Tampering | Excel 单元格→SQL 参数（注入面） | high | mitigate | 全路径 GORM 占位符参数化（repository/fault_info.go 9 处 `?` 占位实证）；单元格值从不拼接 SQL 文本 | closed |
| T-4-13 | Tampering | Excel 公式注入（导入读/未来导出写） | low | accept | v1 无导出端点（需求未列），导入只读单元格值不执行公式；未来加导出时按 -CrLf 前缀防御重评（留档） | closed |
| T-4-SC (04-01) | Tampering | 第三方依赖安装（04-01 零新增） | high | accept | 本 plan 零新增依赖（标准库 + 既有 go.mod）；excelize/minio-go 核查在 04-02/04-04 的 T-4-SC 行 | closed |
| T-4-SC (04-02) | Tampering | go get github.com/minio/minio-go/v7@v7.4.1 | high | mitigate | proxy.golang.org 实测模块 Origin 为 github.com/minio 官方组织（04-RESEARCH 重点 4 版本实证）；go.mod 锁定精确 tag v7.4.1（实证）；无 [ASSUMED]/[SUS] 项 | closed |
| T-4-SC (04-03) | Tampering | 第三方依赖安装（04-03 零新增） | high | accept | 本 plan 零新增依赖（regexp/net 标准库 + 既有栈）；excelize 核查在 04-04 的 T-4-SC 行 | closed |
| T-4-SC (04-04) | Tampering | go get github.com/xuri/excelize/v2@v2.11.0 | high | mitigate | proxy.golang.org 实测模块 Origin 为 github.com/xuri 官方组织（04-RESEARCH 重点 6 版本实证）；go.mod 锁定精确 tag v2.11.0（实证）；无 [ASSUMED]/[SUS] 项 | closed |

*Status: open · closed · open — below high threshold (non-blocking)*
*Severity: critical > high > medium > low — only open threats at or above workflow.security_block_on count toward threats_open*
*Disposition: mitigate (implementation required) · accept (documented risk) · transfer (third-party)*

---

## Accepted Risks Log

| Risk ID | Threat Ref | Rationale | Accepted By | Date |
|---------|------------|-----------|-------------|------|
| AR-4-01 | T-4-01 | dict/config/shift list 无行级过滤：D-03 拍板零 dataScope 应用（需求未要求，通用件已备 v2 挂接）；RequirePerm 功能权限面全量拦截 | plan-time（04-01 威胁建模） | 2026-11-01 |
| AR-4-05 | T-4-05 | 上传内容不扫描：源系统同语义；下载恒 attachment + 全链鉴权 + 单机内网——与源等价残余风险 | plan-time（04-02 威胁建模） | 2026-11-01 |
| AR-4-13 | T-4-13 | Excel 公式注入：v1 无导出端点，导入只读不执行公式；导出落地时重评 | plan-time（04-04 威胁建模） | 2026-11-01 |
| AR-4-SC1 | T-4-SC (04-01/04-03) | 零新增依赖 plan 的供应链面：无安装动作即无新增攻击面（相关依赖核查移至引入 plan） | plan-time（04-01/04-03 威胁建模） | 2026-11-01 |

*Accepted risks do not resurface in future audit runs.*

---

## Security Audit Trail

| Audit Date | Threats Total | Closed | Open | Run By |
|------------|---------------|--------|------|--------|
| 2026-11-01 | 17 | 17 | 0 | claude（gsd-secure-phase L1；asvs=1 + plan-time register + threats_open: 0 短路——grep 级缓解存在性验证 + UAT 实测佐证 6 项） |

---

## Sign-Off

- [x] All threats have a disposition (mitigate / accept / transfer)
- [x] Accepted risks documented in Accepted Risks Log
- [x] `threats_open: 0` confirmed
- [x] `status: verified` set in frontmatter

**Approval:** verified 2026-11-01
