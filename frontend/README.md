# sys-iam-go 管理台前端

单机工控机部署的 sys-iam-go 配套管理台。技术栈 React 19 + antd 5 + Vite 8（照搬自
craft-twin-java 管理台的系统管理域，API 层已适配 Go 后端真实契约：`/api/v1/**`
RESTful、`code=0` 成功信封、`Authorization: Bearer <token>`、分页 `data={items,total}`）。

## 命令

```bash
npm ci          # 严格按 package-lock.json 还原依赖树（零版本重解析）
npm run dev     # 开发服务器（代理 /api → http://localhost:8080）
npm run build   # 产物输出 dist/
npm test        # vitest 全量单测
```

## Go 侧前置

后端按 `configs/config.yaml` 就绪：`server.port` 默认 8080（dev 代理目标），
MySQL（127.0.0.1:3306）与 Redis（127.0.0.1:6379）按其配置运行。
后端启动：仓库根目录 `go run ./cmd/server`；探针 `GET /healthz`、`GET /readyz`。

契约冒烟：`bash scripts/smoke-contract.sh`（真后端直证 healthz / 登录失败 10501 /
无 token 10100 三条契约，纯 curl+grep）。

## 页面

「系统管理」：用户 / 角色 / 菜单 / 组织 / 字典 / 参数配置 / 文件管理；
「日志管理」：操作日志。登录 / 登出 / 权限显隐 / 刷新恢复闭环。

## 已知边界

- 菜单显隐依赖 `system:menu:list`：无该权限的用户登录后左侧菜单为空
  （后端 RequirePerm 仍拦截其实际请求，仅 UX 层影响）。
- 前端权限表派生自 `GET /menus/tree` 树中 permission 非空节点的并集，
  仅 UX 层显隐；安全边界在后端 `requirePerm`（越权请求被 403 拦截）。
