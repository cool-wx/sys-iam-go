// @vitest-environment jsdom
/**
 * 用户管理页组件测试（jsdom）——「系统内置」+ 角色多选 + 角色装配子资源：
 * ① 列表渲染「系统内置」列；内置行删除按钮禁用（后端硬拦截的体验层前置）；
 * ② 新增用户 → 表单含角色多选与系统内置开关；提交后 addUser 收到 roleIds 数字数组 + builtin=true
 *   （附加键直传，后端白名单忽略），且补调 assignUserRoles(新 id, 所选 roleIds)
 *   与 editUser({id, builtin:true})（Go 契约：创建请求体无生效的 roleIds/builtin 键）；
 * ③ 编辑用户 → 角色与内置标志回显；提交后 editUser 收到 roleIds + builtin。
 *
 * iam 模块整体 mock（axios 层与拦截器不在此测）；渲染包一层 antd <App>——
 * App.useApp() 在 <App> 之外拿到的是空对象，message.success 会抛 TypeError 并吞掉其后的刷新调用。
 */
import '@testing-library/jest-dom/vitest'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { App } from 'antd'

const {
  listUsersMock,
  deptTreeMock,
  listRolesMock,
  addUserMock,
  editUserMock,
  assignUserRolesMock,
  deleteUserMock,
  toggleStatusMock,
  resetPwdMock,
} = vi.hoisted(() => ({
  listUsersMock: vi.fn(),
  deptTreeMock: vi.fn(),
  listRolesMock: vi.fn(),
  addUserMock: vi.fn(),
  editUserMock: vi.fn(),
  assignUserRolesMock: vi.fn(),
  deleteUserMock: vi.fn(),
  toggleStatusMock: vi.fn(),
  resetPwdMock: vi.fn(),
}))

vi.mock('../../api/iam.js', () => ({
  listUsers: listUsersMock,
  deptTree: deptTreeMock,
  listRoles: listRolesMock,
  addUser: addUserMock,
  editUser: editUserMock,
  assignUserRoles: assignUserRolesMock,
  deleteUser: deleteUserMock,
  toggleStatus: toggleStatusMock,
  resetPwd: resetPwdMock,
}))

import UserPage from './UserPage.jsx'
import { setPermissions } from '../../utils/perm.js'

const FAKE_DEPT_TREE = [
  { id: 1, deptName: '总部', children: [{ id: 2, deptName: '生产部' }] },
]

const ROLE_BASIC = { id: 2, roleName: '普通用户', roleCode: 'basic' }

/** 两行：鲍勃（普通，可删）+ super（内置，删除禁用） */
const FAKE_USERS = {
  code: 200,
  data: [
    {
      id: 1,
      username: 'bob',
      realName: '鲍勃',
      deptId: 2,
      deptName: '生产部',
      status: 'ENABLED',
      builtin: false,
      roles: [],
    },
    {
      id: 2,
      username: 'super',
      realName: '超管',
      deptId: 1,
      deptName: '总部',
      status: 'ENABLED',
      builtin: true,
      roles: [{ roleId: 2, roleName: '普通用户' }],
    },
  ],
  total: 2,
  message: '成功',
}

function renderPage() {
  return render(
    <App>
      <UserPage />
    </App>,
  )
}

/** 打开指定 label 的 antd Select（rc-select 以 mouseDown 打开；输入框在 selector 容器内） */
function openSelect(labelText) {
  const input = screen.getByLabelText(labelText)
  fireEvent.mouseDown(input.closest('.ant-select-selector') || input)
  return input
}

/**
 * 点选下拉项——选项文本可能与表格行内文本撞车（如行内角色 Tag），
 * 故取全部同文本元素中位于下拉 portal（.ant-select-item-option）内的那个。
 */
async function pickOption(user, labelText) {
  const matches = await screen.findAllByText(labelText)
  const option = matches.find((el) => el.closest('.ant-select-item-option'))
  expect(option, `未找到下拉选项: ${labelText}`).toBeTruthy()
  await user.click(option)
}

describe('UserPage', () => {
  beforeEach(() => {
    setPermissions([
      'system:user:list',
      'system:user:query',
      'system:user:add',
      'system:user:edit',
      'system:user:remove',
      'system:user:resetPwd',
    ])
    listUsersMock.mockReset().mockResolvedValue(FAKE_USERS)
    deptTreeMock.mockReset().mockResolvedValue({ code: 0, data: FAKE_DEPT_TREE, message: 'ok' })
    listRolesMock.mockReset().mockResolvedValue({ code: 0, data: [ROLE_BASIC], total: 1, message: 'ok' })
    addUserMock.mockReset().mockResolvedValue({ code: 0, data: 99, message: 'ok' })
    editUserMock.mockReset().mockResolvedValue({ code: 0, data: null, message: 'ok' })
    assignUserRolesMock.mockReset().mockResolvedValue({ code: 0, data: null, message: 'ok' })
    deleteUserMock.mockReset().mockResolvedValue({ code: 0, data: null, message: 'ok' })
    toggleStatusMock.mockReset().mockResolvedValue({ code: 0, data: null, message: 'ok' })
    resetPwdMock.mockReset().mockResolvedValue({ code: 0, data: null, message: 'ok' })
  })

  it('① 列表渲染系统内置列；内置行删除按钮禁用、普通行可删', async () => {
    renderPage()
    await waitFor(() => expect(screen.getByText('super')).toBeInTheDocument())

    // 内置列：仅 super 行渲染「内置」标记（表头是「系统内置」，不与之等值）
    expect(screen.getByText('内置')).toBeInTheDocument()

    const deleteButtons = screen.getAllByRole('button', { name: /删\s*除/ })
    expect(deleteButtons).toHaveLength(2)
    expect(deleteButtons[0]).not.toBeDisabled() // bob（非内置）
    expect(deleteButtons[1]).toBeDisabled() // super（内置）
  })

  it('② 新增用户 → 选角色 + 打开内置开关 → addUser 直传 roleIds/builtin，并补调角色装配与内置落实', async () => {
    const user = userEvent.setup()
    renderPage()
    await waitFor(() => expect(screen.getByText('bob')).toBeInTheDocument())

    await user.click(screen.getByRole('button', { name: /新增用户/ }))

    await user.type(await screen.findByLabelText('用户名'), 'carol')
    await user.type(screen.getByLabelText('初始密码'), '123456')

    openSelect('部门')
    await pickOption(user, '生产部')

    openSelect('角色')
    await pickOption(user, '普通用户')

    const switchEl = screen.getByRole('switch')
    expect(switchEl).not.toBeChecked()
    await user.click(switchEl)

    await user.click(screen.getByRole('button', { name: /^保\s*存$/ }))

    await waitFor(() => expect(addUserMock).toHaveBeenCalledTimes(1))
    expect(addUserMock).toHaveBeenCalledWith(
      expect.objectContaining({ username: 'carol', roleIds: [2], builtin: true }),
    )
    // Go 契约补充装配：创建请求体的 roleIds/builtin 键被白名单忽略——
    // 角色经子资源 PUT /users/:id/roles（新 id=99 + 所选 roleIds）；
    // 内置标志经 PUT /users/:id {builtin:true} 落实
    await waitFor(() => expect(assignUserRolesMock).toHaveBeenCalledTimes(1))
    expect(assignUserRolesMock).toHaveBeenCalledWith(99, [2])
    await waitFor(() => expect(editUserMock).toHaveBeenCalledTimes(1))
    expect(editUserMock).toHaveBeenCalledWith({ id: 99, builtin: true })
  })

  it('③ 编辑内置用户 → 角色与内置标志回显 → editUser 收到 roleIds 与 builtin', async () => {
    const user = userEvent.setup()
    renderPage()
    await waitFor(() => expect(screen.getByText('super')).toBeInTheDocument())

    const editButtons = screen.getAllByRole('button', { name: /^编\s*辑$/ })
    await user.click(editButtons[1]) // super 行

    await screen.findByText('编辑用户：super')
    // 角色多选回显（selection item 文本在 .ant-select 容器内）
    await waitFor(() =>
      expect(screen.getByLabelText('角色').closest('.ant-select').textContent).toContain('普通用户'),
    )
    expect(screen.getByRole('switch')).toBeChecked()

    await user.click(screen.getByRole('button', { name: /^保\s*存$/ }))

    await waitFor(() => expect(editUserMock).toHaveBeenCalledTimes(1))
    expect(editUserMock).toHaveBeenCalledWith(
      expect.objectContaining({ id: 2, roleIds: [2], builtin: true }),
    )
  })
})
