/**
 * 路由聚合器（静态路由）。
 *
 * import.meta.glob 收集各域 pages/<domain>/routes.jsx 导出的 PAGES 数组
 * （{ path, label, perm, icon, group, element }）——各域只维护自己的 routes.jsx，
 * 本文件零触碰。PAGES 同时是 Sider 菜单 / 面包屑的数据源。
 * / 重定向 /system/users（登录后默认落在用户管理——本管理台的第一个业务页）。
 */
import { Navigate, Route, Routes } from 'react-router'
import AuthGuard from './AuthGuard.jsx'
import Layout from './Layout.jsx'
import LoginPage from '../pages/login/LoginPage.jsx'

const modules = import.meta.glob('/src/pages/*/routes.jsx', { eager: true })

/** 全站页面注册表：{ path, label, perm, icon, group, element }[] */
export const PAGES = Object.values(modules).flatMap((m) =>
  Array.isArray(m.PAGES) ? m.PAGES : [],
)

export function AppRoutes() {
  return (
    <Routes>
      {/* 登录页独立于守卫（无 token 入口） */}
      <Route path="/login" element={<LoginPage />} />
      {/* 其余页面经 AuthGuard 包 Layout（无 token 重定向 /login） */}
      <Route element={<AuthGuard />}>
        <Route path="/" element={<Layout />}>
          <Route index element={<Navigate to="/system/users" replace />} />
          {PAGES.map((p) => (
            <Route key={p.path} path={p.path.replace(/^\//, '')} element={p.element} />
          ))}
        </Route>
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )
}
