import { FileSearchOutlined } from '@ant-design/icons'
import OperationLogPage from './OperationLogPage.jsx'

/**
 * 日志管理页面注册表（被 src/router/routes.jsx 的 import.meta.glob 聚合）。
 * 权限标识与后端 RequirePerm 常量逐字对齐：GET /api/v1/logs 单一权限点
 * system:log:list（internal/authz/perms.go PermLogList）。
 */
export const PAGES = [
  {
    path: '/logs/operation',
    label: '操作日志',
    perm: 'system:log:list',
    icon: <FileSearchOutlined />,
    group: '日志管理',
    element: <OperationLogPage />,
  },
]
