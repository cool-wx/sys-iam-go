---
status: complete
phase: 02-rbac
source: [live-db-end-to-end]
started: 2026-11-11T07:35:00Z
updated: 2026-11-11T08:29:33.598Z
---

## Current Test

[testing complete]

## Tests

### 1. Cold Start Smoke Test
expected: 服务启动连本机库，boot 对账填充 casbin_rule（g+p 行），/healthz 200
result: pass
note: 服务以 -config configs/config.yaml 启动，连 127.0.0.1:3306/sys-iam-go + 127.0.0.1:6379（db0），healthz=200 readyz=200 无致命错误

### 2. 正确凭据登录
expected: uat_super POST /api/v1/auth/login 返回 200，含 token 与用户信息
result: pass
note: http=200 code=0，token 64 位，realName=UAT超管 status=ENABLED

### 3. 错误密码登录拒绝
expected: 错误密码返回业务错误，不发放 token
result: pass
note: http=401 code=10501「用户名或密码错误」

### 4. 无 token 访问受保护路由被拒
expected: 不带 token GET /api/v1/users 返回 401
result: pass
note: http=401 code=10100「未登录」

### 5. super_admin 用户列表
expected: GET /api/v1/users 返回 200 分页数据，含真实用户
result: pass
note: 首测 total=0——根因为存量数据缺口：super_admin 角色 data_scope=1（ModeSelf 本人档，仅见 create_by_id=自己的行），非代码缺陷；datascope 注释明确「存量行由现场经 PUT /roles/:id 修正」。经 PUT /api/v1/roles/1 {"dataScope":5} 修正后复测 total=7（super/hxl/wx/admin/uat_super/uat_admin/uat_basic 全量可见）

### 6. 用户创建与删除闭环
expected: POST 创建 uat_tmp_user 成功；GET :id 可见；DELETE 后 GET 404；不留残留
result: pass
note: 首测 400「部门不能为空」（deptId 服务端必填校验，符合设计）；带 deptId=1 重测：create 200（雪花 ID 2120433539479506944，deptName 服务端装配）→ get 200 → delete 200 → get 404 → 库无残留行

### 7. 角色列表
expected: GET /api/v1/roles 返回 200，含三角色
result: pass
note: super_admin(dataScope=5)/admin/basic 齐备，builtin 标记正确

### 8. 菜单树
expected: GET /api/v1/menus/tree 返回 200 树结构
result: pass
note: http=200 树结构（parentId=0 根节点起）

### 9. 部门树
expected: GET /api/v1/depts/tree 返回 200 树结构
result: pass
note: http=200（物流科等 3 部门）

### 10. basic 角色权限拦截（403）
expected: uat_basic（零菜单授权）GET /api/v1/users 返回 403
result: pass
note: http=403 code=10200「无权限」

### 11. 发明串权限域仅 super_admin
expected: uat_admin GET /api/v1/shifts 403；uat_super 200（matcher 豁免）
result: pass
note: admin_shifts=403（system:shift:list 无菜单项），super_shifts=200

### 12. 字典域读取
expected: GET /api/v1/dict-types 与 /dict-datas 返回 200
result: pass
note: types 200（设备类型 device_type），datas 200（13 条）

### 13. 故障域分页
expected: uat_super GET /api/v1/faults 返回 200 分页
result: pass
note: http=200 默认 1/15（存量 196 条）

### 14. 操作日志落库
expected: 登录/创建操作后 GET /api/v1/logs 可查记录
result: pass
note: logs 查询 200；近 10 分钟 module=AuthController/UserController 日志 3 条（login/create 等动作均有落库）

### 15. casbin boot 对账行数
expected: 启动后 casbin_rule g 行与 p 行非零
result: pass
note: 启动前 0 行 → 启动后 total=317（g=7 p=310），boot 对账从 sys_user_role/sys_role_menu 重建成功

### 16. 登出后会话失效
expected: POST /api/v1/auth/logout 成功后，同 token 再访问返回 401
result: pass
note: logout 200 → 同 token GET /api/v1/users 401 code=10100

## Summary

total: 16
passed: 16
issues: 0
pending: 0
skipped: 0
blocked: 0

## Gaps

[none]

## Test Data Changes (UAT 期间按用户授权添加/修正的数据)

- 添加测试账号：uat_super(990001→super_admin)/uat_admin(990002→admin)/uat_basic(990003→basic)，密码 Uat#2026，remark 标记「UAT 测试账号-自动添加」
- 修正存量数据缺口：sys_role id=1（super_admin）data_scope 1→5（经 PUT /api/v1/roles/1，datascope 代码注释指定的现场修正路径；update_by=uat_super 留审计）
- 闭环清理：uat_tmp_user 已删除无残留；服务进程已停止
