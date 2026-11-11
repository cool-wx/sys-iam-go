// @vitest-environment jsdom
/**
 * 角色管理页组件测试（jsdom）：
 * ① 列表渲染角色行（CrudTable 分页口径）；
 * ② 打开分配抽屉 → 加载菜单树 + 回显勾选（叶子口径，父级半选）；
 * ③ 保存分配 → assignRoleMenus 收到勾选 id 数组（叶子 + 祖先全集）。
 * iam 模块 mock 掉（axios 层与拦截器不在此测）。
 */
import '@testing-library/jest-dom/vitest'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { App } from 'antd'

const {
  listRolesMock,
  getRoleMenusMock,
  assignRoleMenusMock,
  menuTreeMock,
  addRoleMock,
  editRoleMock,
  deleteRoleMock,
} = vi.hoisted(() => ({
  listRolesMock: vi.fn(),
  getRoleMenusMock: vi.fn(),
  assignRoleMenusMock: vi.fn(),
  menuTreeMock: vi.fn(),
  addRoleMock: vi.fn(),
  editRoleMock: vi.fn(),
  deleteRoleMock: vi.fn(),
}))
vi.mock('../../api/iam.js', () => ({
  listRoles: listRolesMock,
  getRoleMenus: getRoleMenusMock,
  assignRoleMenus: assignRoleMenusMock,
  menuTree: menuTreeMock,
  addRole: addRoleMock,
  editRole: editRoleMock,
  deleteRole: deleteRoleMock,
}))

import RolePage from './RolePage.jsx'
import { setPermissions } from '../../utils/perm.js'

// 菜单树 fake：两层嵌套（10 为父，11/12 叶子；20 为无子级根节点）
const FAKE_TREE = [
  {
    id: 10,
    name: '系统管理',
    children: [
      { id: 11, name: '用户管理' },
      { id: 12, name: '角色管理' },
    ],
  },
  { id: 20, name: '首页' },
]

const FAKE_ROLES = {
  code: 200,
  data: [
    { id: 1, roleName: '管理员', roleCode: 'admin', status: 'ENABLED', remark: '全部权限', builtin: false },
  ],
  total: 1,
  message: '成功',
}

/** 261014-kg7：一行内置（super_admin）+ 一行普通——用于内置列与删除守卫用例 */
const FAKE_ROLES_WITH_BUILTIN = {
  code: 200,
  data: [
    {
      id: 1,
      roleName: '超级管理员',
      roleCode: 'super_admin',
      status: 'ENABLED',
      remark: '系统内置',
      builtin: true,
    },
    {
      id: 2,
      roleName: '普通用户',
      roleCode: 'basic',
      status: 'ENABLED',
      remark: '只读',
      builtin: false,
    },
  ],
  total: 2,
  message: '成功',
}

/** 渲染包一层 antd <App>——App.useApp() 在 <App> 之外是空对象，message.success 会抛 TypeError */
function renderPage() {
  return render(
    <App>
      <RolePage />
    </App>,
  )
}

describe('RolePage', () => {
  beforeEach(() => {
    // PermButton 无权限即隐藏——测试注入全量角色权限
    setPermissions([
      'system:role:list',
      'system:role:query',
      'system:role:add',
      'system:role:edit',
      'system:role:remove',
    ])
    listRolesMock.mockReset().mockResolvedValue(FAKE_ROLES)
    // 回显只勾一个子（11）+ 无子级根（20）——父 10 应推导为半选
    getRoleMenusMock.mockReset().mockResolvedValue({ code: 200, data: [11, 20], message: '成功' })
    assignRoleMenusMock.mockReset().mockResolvedValue({ code: 200, data: null, message: '成功' })
    menuTreeMock.mockReset().mockResolvedValue({ code: 200, data: FAKE_TREE, message: '成功' })
    addRoleMock.mockReset().mockResolvedValue({ code: 200, data: 9, message: '成功' })
    editRoleMock.mockReset().mockResolvedValue({ code: 200, data: null, message: '成功' })
    deleteRoleMock.mockReset().mockResolvedValue({ code: 200, data: null, message: '成功' })
  })

  it('① 列表渲染角色行（角色名/编码出现）', async () => {
    renderPage()
    await waitFor(() => expect(listRolesMock).toHaveBeenCalledWith({ page: 1, size: 20 }))
    await waitFor(() => expect(screen.getByText('管理员')).toBeInTheDocument())
    expect(screen.getByText('admin')).toBeInTheDocument()
  })

  it('② 打开分配抽屉 → 加载菜单树 + 回显勾选（叶子勾选、父级半选）', async () => {
    const user = userEvent.setup()
    renderPage()
    await waitFor(() => expect(screen.getByText('管理员')).toBeInTheDocument())

    await user.click(screen.getByRole('button', { name: /分\s*配\s*菜\s*单/ }))

    await waitFor(() => expect(menuTreeMock).toHaveBeenCalled())
    await waitFor(() => expect(getRoleMenusMock).toHaveBeenCalledWith(1))
    // 树节点渲染：系统管理(父) / 用户管理 / 角色管理 / 首页
    await waitFor(() => expect(screen.getByText('用户管理')).toBeInTheDocument())
    const boxes = screen.getAllByRole('checkbox')
    expect(boxes).toHaveLength(4)
    expect(boxes[0]).toBePartiallyChecked() // 系统管理（仅 11 勾选 → 半选）
    expect(boxes[1]).toBeChecked() // 用户管理（回显勾选）
    expect(boxes[2]).not.toBeChecked() // 角色管理（未分配）
    expect(boxes[3]).toBeChecked() // 首页
  })

  it('③ 保存分配 → assignRoleMenus 收到勾选 id 数组（叶子 + 祖先）', async () => {
    const user = userEvent.setup()
    renderPage()
    await waitFor(() => expect(screen.getByText('管理员')).toBeInTheDocument())

    await user.click(screen.getByRole('button', { name: /分\s*配\s*菜\s*单/ }))
    await waitFor(() => expect(screen.getByText('用户管理')).toBeInTheDocument())

    await user.click(screen.getByRole('button', { name: /^保\s*存$/ }))

    await waitFor(() => expect(assignRoleMenusMock).toHaveBeenCalled())
    const [idArg, menuIds] = assignRoleMenusMock.mock.calls[0]
    expect(idArg).toBe(1)
    expect(menuIds).toHaveLength(3)
    expect(menuIds).toEqual(expect.arrayContaining([10, 11, 20])) // 11 叶子 + 10 祖先 + 20 叶子
  })

  it('④ 内置角色行删除按钮禁用；普通行删除链路零回归（Popconfirm 确认后 deleteRole）', async () => {
    const user = userEvent.setup()
    listRolesMock.mockResolvedValue(FAKE_ROLES_WITH_BUILTIN)
    renderPage()
    await waitFor(() => expect(screen.getByText('超级管理员')).toBeInTheDocument())

    // 「系统内置」列：仅内置行渲染「内置」标记
    expect(screen.getByText('内置')).toBeInTheDocument()

    const deleteButtons = screen.getAllByRole('button', { name: /删\s*除/ })
    expect(deleteButtons).toHaveLength(2)
    expect(deleteButtons[0]).toBeDisabled() // super_admin（内置）
    expect(deleteButtons[1]).not.toBeDisabled() // basic（普通）

    await user.click(deleteButtons[1])
    await user.click(await screen.findByRole('button', { name: /^确\s*认$/ }))
    await waitFor(() => expect(deleteRoleMock).toHaveBeenCalledWith(2))
  })

  it('⑤ 表单含系统内置开关；编辑内置角色回显为开并提交 builtin', async () => {
    const user = userEvent.setup()
    listRolesMock.mockResolvedValue(FAKE_ROLES_WITH_BUILTIN)
    renderPage()
    await waitFor(() => expect(screen.getByText('超级管理员')).toBeInTheDocument())

    // 新增：表单含系统内置开关且默认关
    await user.click(screen.getByRole('button', { name: /新增角色/ }))
    expect(await screen.findByRole('switch')).not.toBeChecked()
    await user.click(screen.getByRole('button', { name: /^取\s*消$/ }))

    // 编辑内置角色：开关回显为开
    await user.click(screen.getAllByRole('button', { name: /^编\s*辑$/ })[0])
    await screen.findByText('编辑角色：超级管理员')
    await waitFor(() => expect(screen.getByRole('switch')).toBeChecked())

    await user.click(screen.getByRole('button', { name: /^保\s*存$/ }))
    await waitFor(() => expect(editRoleMock).toHaveBeenCalled())
    expect(editRoleMock).toHaveBeenCalledWith(expect.objectContaining({ id: 1, builtin: true }))
  })
})
