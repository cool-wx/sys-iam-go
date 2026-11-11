import { Navigate, Outlet } from 'react-router'
import { getToken } from '../stores/tokenStorage.js'

/**
 * 路由守卫（D-07）：无 token → 重定向 /login；有 token → 渲染子路由。
 * 前端仅 UX 层入口控制，安全边界在后端 @SaCheckPermission。
 */
export default function AuthGuard() {
  if (!getToken()) return <Navigate to="/login" replace />
  return <Outlet />
}
