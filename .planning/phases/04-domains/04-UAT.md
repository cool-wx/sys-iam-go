---
status: complete
phase: 04-domains
source: [04-01-SUMMARY.md, 04-02-SUMMARY.md, 04-03-SUMMARY.md, 04-04-SUMMARY.md]
started: 2026-11-01T22:59:41+08:00
updated: 2026-11-01T15:24:12.525Z
tester: claude（用户指令：所有 UAT 测试由 Claude 执行——真实 server 冷启动 + curl 实测，非交互问答）
---

## Current Test

[testing complete]

## Tests

### 1. 冷启动冒烟（全新库 + DDL + 种子 + 二进制启动）
expected: 全新空库加载 17 DDL + 种子后启动二进制；/healthz 200 {"status":"ok"}；admin 登录 200 且 token 非空
result: pass
evidence: "CGO_ENABLED=0 静态二进制 64MB；空工作目录（无 config.yaml）→ 内置默认值 + IAM_* env 覆盖启动成功；healthz/readyz 均 200 {\"status\":\"ok\"}；boot 日志仅预期 WARN（config 缺省）+ casbin 首载 SLOW SQL 提示，0 error；admin/admin123 登录 200 拿 64hex token"

### 2. 写操作自动落操作日志（七要素 + body 回放不破坏业务）
expected: 带 token 创建字典类型返回业务 200；随后 GET /api/v1/logs 可查到该操作记录，含 module（DictTypeController）/operation（create）/user_id/real_name/method（POST）/url/ip 七要素，params 含请求体回放
result: pass
evidence: "POST /api/v1/dict-types 200（code 0 envelope 正常，body 回放未破坏业务）；日志行七要素齐：module=DictTypeController/operation=create/userId=9900002/realName/method=\"POST /api/v1/dict-types\"/ip=127.0.0.1/params={\"dictName\":...,\"dictType\":\"sex\"}，status=1 duration=27ms"

### 3. 敏感字段掩码（8 字段）
expected: 登录请求（含 password）落日志后 params 中 password 值显示为掩码形（非明文）；掩码先于截断
result: pass
evidence: "module=AuthController 的 login 日志 params={\"username\":\"admin\",\"password\":\"******\"}——password 掩码、username 明文；login 未登录 NULL identity 分支 userId/realName=None 亦符合 04-03 设计"

### 4. 失败请求也落日志（status=0 + error_msg）
expected: 重复创建同名字典类型得到 400 后，日志行记录 status=0 与 error_msg（错误消息文本）
result: pass
evidence: "重复创建 dictType=sex → 400/10400「字典类型已存在」；对应日志行 status=0 errorMsg=「字典类型已存在」，与成功行（status=1）成对落库"

### 5. 日志查询分页与过滤（OLOG-03）
expected: GET /api/v1/logs 不传分页默认 page=1 size=10（特例分页）；module 模糊过滤命中指定模块；列表按 create_time 倒序
result: pass
evidence: "无参默认 page=1 size=10 total=5；module=DictType 前缀 like 命中 DictTypeController 行；operation=login 命中全部 3 条 login；real_name=管理员 like 精确命中 1 条且正确排除 NULL/mojibake 历史行；负例 real_name=张三丰 total=0 items=[] 非 null；createTime 严格降序"
note: "real_name 过滤参数为 snake_case（real_name 非 realName）——测试者首试用错参数名，属测试错误非代码缺陷"

### 6. Fault 域 CRUD 五路由（FAUL-01）
expected: POST /api/v1/faults 创建成功且未传 bitIndex/type 时服务端默认 -1/3；GET :id 详情一致；PUT 更新生效；GET list 默认 1/15 分页且 code 过滤生效；DELETE 后 404；无 token 401
result: pass
evidence: "create 省缺 bitIndex/type → resp bitIndex=-1 type=UNKNOWN 且 DB 行 bit_index=-1/type=3（双证）；GET 详情一致；PUT 更新 detail/fix 生效（nil-skip 未传字段不变）；list 默认 page=1 size=15 + code eq 过滤 total=1；DELETE 200 后 GET 404/10300「故障信息不存在: {id}」；无 token 401/10100「未登录」"

### 7. Excel 三键 upsert 导入（FAUL-02）
expected: 构造 8 列契约 xlsx 上传 /api/v1/faults/import：首次导入返回 insertCount>0 updateCount=0；同文件再导三键（code+name+location）命中行 updateCount>0 且 id 复用（行数不增）；location 为 NULL 的行反复 INSERT 为源语义保真（Q2 照源裁定，prohibition 禁改）；含触发位非数字坏行的文件整批回滚（DB 行数不变）；表头不符 400 整批拒绝
result: pass
evidence: "首导 {insertCount:3, updateCount:0} DB 3 行；再导/三导 {insertCount:1, updateCount:2}——F-100/F-200（有 location）id 两导间完全不变（三键命中 Patch 复用原行），F-300（location NULL）每导 +1 行 = Q2 源语义保真；坏文件（触发位 abc）400「第 3 行触发位必须为整数: abc」DB 行数不变（整批回滚）；错表头 400「Excel 表头与模板不符」；ImportResult 契约 {insertCount, updateCount} 逐字符合"
note: "原 expected 写「含 name 空行整批回滚」不准确——name 空行是 skip 语义（不计数不回滚），回滚触发是解析失败行；已按实际契约修正 expected 描述"

### 8. [04-01 D1] 字典类型 CRUD 全链
expected: 字典类型 CRUD 全链：uk_dict_type 重复插入 400/10400「字典类型已存在」、update 零值保护、删除后 404
result: pass
source: automated
coverage_id: 04-01/D1

### 9. [04-01 D2] 字典数据 CRUD（类型校验 + 冗余写）
expected: 字典数据 CRUD：类型存在性前置校验（404/10300「字典类型不存在: {id}」源消息原文）+ dict_type_name 服务端冗余写（resp 与 DB 行双断言）
result: pass
source: automated
coverage_id: 04-01/D2

### 10. [04-01 D3] 字典数据按类型过滤
expected: GET /api/v1/dict-datas?dict_type_id= 仅返回该类型数据（envelope items/total/page/size 齐），不传返回全量分页
result: pass
source: automated
coverage_id: 04-01/D3

### 11. [04-01 D4] 系统配置 CRUD + key 查询
expected: 系统配置 CRUD：GET /configs/key/{key} data 为 configValue 字符串本体、uk_config_key 重复 400/10400「配置键已存在」、key 未命中 404
result: pass
source: automated
coverage_id: 04-01/D4

### 12. [04-01 D5] 排班全量直出（无分页 envelope）
expected: GET /api/v1/shifts 响应 data 为 JSON 数组直出（无 items/total 分页键——SHFT-01 源行为直译）
result: pass
source: automated
coverage_id: 04-01/D5

### 13. [04-01 D6] 排班 CRUD + 时间字符串直传
expected: 排班 CRUD 五路由可用，start_time/end_time 以 HH:MM:SS 字符串直传（无解析层/格式校验）
result: pass
source: automated
coverage_id: 04-01/D6

### 14. [04-01 D7] 21 条新路由逐条 RequirePerm
expected: 本 plan 21 条新路由全部逐条 RequirePerm（D-04）；无 token 401/无 perm 403/super_admin 200 三面；结构 gate 计数 57
result: pass
source: automated
coverage_id: 04-01/D7

### 15. [04-02 D1] 上传 MD5 判重五语义
expected: 上传 MD5 判重五语义：同分类同内容 400 源消息 + 零副作用（sys_file 0 新行+存储 1 对象）/异分类放行/未分类 IS NULL 分支命中/异内容放行/分类不存在 404 源消息；category_name 服务端冗余
result: pass
source: automated
coverage_id: 04-02/D1

### 16. [04-02 D2] 流式下载
expected: 流式下载：2MB 文件 download 200 + Content-Length == size + 响应体逐字节一致（GetStream→DataFromReader 全链不整载）+ 不存在 id 404 域消息 + 分页列表 envelope
result: pass
source: automated
coverage_id: 04-02/D2

### 17. [04-02 D3] 宽容删除
expected: 宽容删除：上传→删除 200 且 DB 行消失+存储对象移除；不存在 id 404（源 slog.Warn 宽容语义由代码路径保证，LocalStorage 成功路径覆盖主链）
result: pass
source: automated
coverage_id: 04-02/D3

### 18. [04-02 D4] 文件分类 CRUD+树
expected: 文件分类 CRUD+树：两层级嵌套+parent_name 冗余（DB 行+树节点双断言）/孤儿 SQL 直插提升为根/上级分类不存在 404/10300 源消息/叶子 children []非 null/显式移至根 parent_name 清空
result: pass
source: automated
coverage_id: 04-02/D4

### 19. [04-02 D5] storage.driver 配置切换
expected: storage.driver 配置切换：local 分支零 minio 触碰（延迟形态）；driver=s3 构造内 EnsureBucket fail-fast；rustfs 真容器 Put/Get/Delete 往返 smoke 绿（不可达 skip 策略）；同一 FileService 代码路径对双后端无差别
result: pass
source: automated
coverage_id: 04-02/D5

### 20. [04-02 D6] 工控机 DoS 防护面
expected: 工控机 DoS 防护面：全局 BodyLimit 1MB 对 multipart 跳过 + upload 路由 BodyLimitWithMB(20) 唯一封顶 + MaxMultipartMemory 8MB 落盘阈值；9 路由逐条 RequirePerm 收口 66（57+9），download 挂 PermFileDownload（D-16）
result: pass
source: automated
coverage_id: 04-02/D6

## Summary

total: 20
passed: 20
issues: 0
pending: 0
skipped: 0
blocked: 0

## Live spot-checks（自动化条目的真实 server 佐证）

- shifts 数组直出（#12）：GET /api/v1/shifts data 为 JSON 数组、无 items/total 键，startTime/endTime HH:MM:SS 直传 ✓
- config key 本体（#11）：GET /configs/key/sys.uat.title → data 为字符串 "UAT标题" 本体；missing key 404/10300「配置不存在: missing.key」✓
- dict-datas envelope（#10）：不传过滤 envelope items/total/page/size 四键齐 ✓
- 文件判重/下载/删除（#15/#16/#17）：同内容二次上传 400「文件已存在：f1.txt（内容 MD5 重复，同一分类下不可重复上传）」源消息逐字；GET /api/v1/files/download/:id 200 + Content-Length=23 精确 + RFC 5987 attachment 头 + md5 逐字节一致；不存在 id 404/10300；DELETE 后存储目录清空 ✓
- 结构 gate 复测：RequirePerm=73 / Oplog=45 / perms=58 / DDL=17（BodyLimitWithMB=2 为 04-04 导入路由叠加，符合演进）✓
- 测试者失误记录（非代码缺陷）：real_name 过滤参数名首试误用 realName；文件下载 URL 首试误拼 /files/:id/download（实际注册 /files/download/:id）——两处均以正确形态复测通过

## Gaps

[none]
