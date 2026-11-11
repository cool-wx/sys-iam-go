/**
 * 登录态全局状态（zustand，D-04）。
 *
 * loginSuccess：登录/刷新恢复时一次性写入 user + permissions，
 * 并同步进 perm 模块（utils/perm.js 模块级 Set——hasPerm/PermButton/菜单过滤的数据源）。
 * logout：清 state + 清 token（perm 模块同步清）。
 */
import { create } from 'zustand'
import { setPermissions, clearPermissions } from '../utils/perm.js'
import { clearToken } from './tokenStorage.js'

export const useAuthStore = create((set) => ({
  user: null,
  permissions: [],

  /** 登录成功 / 刷新后恢复会话时调用 */
  loginSuccess: (user, permissions) => {
    setPermissions(permissions)
    set({ user: user || null, permissions: Array.isArray(permissions) ? permissions : [] })
  },

  /** 退出登录：清 state + 清 token + 清权限表 */
  logout: () => {
    clearPermissions()
    clearToken()
    set({ user: null, permissions: [] })
  },
}))
