// @vitest-environment jsdom
/**
 * PermButton 组件测试（quick 261014-m98 活体验证补测）。
 *
 * ③ 是**缺陷守门用例**：权限表晚于首帧到达时，按钮必须补渲。
 * 背景——`PAGES[].element` 是模块级常量元素，Layout 因权限更新重渲时
 * `<Outlet/>` 返回同一元素引用 → React 对同引用子树 **bailout 跳过重渲**，
 * 故 PermButton 不能依赖父组件重渲，必须自身订阅权限状态。
 * 该缺陷曾致硬刷新直达业务页时「操作」列整列空白（需手点刷新才出现）。
 */
import '@testing-library/jest-dom/vitest'
import { describe, it, expect, beforeEach } from 'vitest'
import { render, screen, act } from '@testing-library/react'
import PermButton from './PermButton.jsx'
import { useAuthStore } from '../stores/authStore.js'
import { setPermissions, clearPermissions } from '../utils/perm.js'

describe('PermButton', () => {
  beforeEach(() => {
    clearPermissions()
    useAuthStore.setState({ user: null, permissions: [] })
  })

  it('① 有权限 → 渲染按钮并透传 props', () => {
    setPermissions(['system:user:resetPwd'])
    render(
      <PermButton perm="system:user:resetPwd" danger>
        重置密码
      </PermButton>,
    )
    expect(screen.getByRole('button', { name: '重置密码' })).toBeInTheDocument()
  })

  it('② 无权限 → 渲染 null（整按钮隐藏）', () => {
    setPermissions(['system:role:remove'])
    render(<PermButton perm="system:user:resetPwd">重置密码</PermButton>)
    expect(screen.queryByRole('button')).toBeNull()
  })

  it('③ 权限后到 → 补渲（不依赖父组件重渲）', () => {
    // 首帧：权限表为空（硬刷新进入页面的真实时序）
    render(<PermButton perm="system:user:resetPwd">重置密码</PermButton>)
    expect(screen.queryByRole('button')).toBeNull()

    // 权限表到达（Layout 的权限派生返回后调 loginSuccess）
    act(() => {
      useAuthStore.getState().loginSuccess({ username: 'admin' }, ['system:user:resetPwd'])
    })

    expect(screen.getByRole('button', { name: '重置密码' })).toBeInTheDocument()
  })
})
