/**
 * 登录态全局状态（zustand）。
 *
 * loginSuccess：登录/刷新恢复时一次性写入 user + permissions，
 * 并同步进 perm 模块（utils/perm.js 模块级 Set——hasPerm/PermButton/菜单过滤的数据源）；
 * user 非空时同步落 sessionStorage 快照（tokenStorage USER_KEY——刷新后昵称恢复，
 * Go 后端无用户信息端点；user 为 null 的重派生调用不覆盖已有快照）。
 * logout：清 state + 清 token（clearToken 级联清用户快照；perm 模块同步清）。
 */
import { create } from 'zustand'
import { setPermissions, clearPermissions } from '../utils/perm.js'
import { clearToken, saveUserSnapshot } from './tokenStorage.js'

/** 空权限表共享常量：空数组恒用同一引用——消费方以 permissions 为 effect 依赖时
 * （Layout 派生 effect），「无权限」的多次写入不得产生新引用触发重跑。 */
const EMPTY_PERMS = []

export const useAuthStore = create((set) => ({
  user: null,
  permissions: EMPTY_PERMS,

  /** 登录成功 / 刷新后恢复会话时调用 */
  loginSuccess: (user, permissions) => {
    setPermissions(permissions)
    saveUserSnapshot(user)
    set({
      user: user || null,
      permissions: Array.isArray(permissions) && permissions.length ? permissions : EMPTY_PERMS,
    })
  },

  /** 退出登录：清 state + 清 token + 清权限表 */
  logout: () => {
    clearPermissions()
    clearToken()
    set({ user: null, permissions: EMPTY_PERMS })
  },
}))
