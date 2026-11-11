// @vitest-environment jsdom
/**
 * 登录页组件测试（jsdom）：
 * ① 提交成功 → tokenStorage 写入 iam.token + 同一响应取 user + menuPermissions 预拉进权限表 + 跳转 '/';
 * ② 提交失败（登录被拒）→ 无 token 写入、不跳转。
 * iam 模块 mock 掉（axios 层与拦截器不在此测——resolveR 纯函数另有 httpResolve.test.js 锁）。
 */
import '@testing-library/jest-dom/vitest'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

const { loginMock, menuPermissionsMock, navigateMock } = vi.hoisted(() => ({
  loginMock: vi.fn(),
  menuPermissionsMock: vi.fn(),
  navigateMock: vi.fn(),
}))

vi.mock('../../api/iam.js', () => ({
  login: loginMock,
  menuPermissions: menuPermissionsMock,
}))

vi.mock('react-router', async (importOriginal) => {
  const actual = await importOriginal()
  return { ...actual, useNavigate: () => navigateMock }
})

import LoginPage from './LoginPage.jsx'
import { getToken, clearToken } from '../../stores/tokenStorage.js'
import { useAuthStore } from '../../stores/authStore.js'
import { hasPerm, clearPermissions } from '../../utils/perm.js'

function renderPage() {
  return render(<LoginPage />)
}

async function fillAndSubmit(user, username, password) {
  await user.type(screen.getByLabelText('用户名'), username)
  await user.type(screen.getByLabelText('密码'), password)
  await user.click(screen.getByRole('button', { name: /登\s*录/ }))
}

describe('LoginPage', () => {
  beforeEach(() => {
    loginMock.mockReset()
    menuPermissionsMock.mockReset()
    navigateMock.mockReset()
    clearToken()
    clearPermissions()
    useAuthStore.setState({ user: null, permissions: [] })
  })

  it('① 登录成功 → iam.token 写入 + 同一响应 user 进 store + menuPermissions 预拉进权限表 + 跳转 /', async () => {
    const user = userEvent.setup()
    loginMock.mockResolvedValue({
      code: 0,
      data: { token: 't1', user: { username: 'admin', realName: '管理员' } },
      message: 'ok',
    })
    menuPermissionsMock.mockResolvedValue(['system:user:list'])
    renderPage()

    await fillAndSubmit(user, 'admin', 'admin123')

    await waitFor(() => expect(getToken()).toBe('t1'))
    await waitFor(() => expect(menuPermissionsMock).toHaveBeenCalledTimes(1))
    await waitFor(() => expect(hasPerm('system:user:list')).toBe(true))
    await waitFor(() =>
      expect(useAuthStore.getState().user).toEqual({ username: 'admin', realName: '管理员' }),
    )
    await waitFor(() => expect(navigateMock).toHaveBeenCalledWith('/', { replace: true }))
    expect(loginMock).toHaveBeenCalledWith({ username: 'admin', password: 'admin123' })
  })

  it('② 登录失败（用户名或密码错误）→ 无 token 写入、不跳转', async () => {
    const user = userEvent.setup()
    loginMock.mockRejectedValue(new Error('用户名或密码错误'))
    renderPage()

    await fillAndSubmit(user, 'admin', 'wrong')
    await waitFor(() => expect(loginMock).toHaveBeenCalled())

    expect(getToken()).toBeNull()
    expect(menuPermissionsMock).not.toHaveBeenCalled()
    expect(navigateMock).not.toHaveBeenCalled()
  })
})
