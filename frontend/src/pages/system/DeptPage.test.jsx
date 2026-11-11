// @vitest-environment jsdom
/**
 * 组织管理页组件测试（jsdom）：
 * ① 树形表格渲染交付机同形数据（物流科 → 成型班组），受控展开生效；
 * ② 无 system:dept:list 时页面仍可挂载不崩（权限过滤仅作用于按钮显隐）；
 * ③ 新增：填部门名 → 保存 → addDept 收到 objectContaining({deptName}) 且列表刷新；
 * ④ 编辑：回显该行 deptName → 保存 → editDept 收到 objectContaining({id, deptName})；
 * ⑤ 删除守卫：有子级节点点删除零请求；叶子节点 Popconfirm 确认后 deleteDept(id)；
 * ⑥ 权限显隐：仅 list 时「新增部门」不存在，补 add 后出现。
 * iam 模块 mock 掉（axios 层与拦截器不在此测）。
 */
import '@testing-library/jest-dom/vitest'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { App } from 'antd'

const { deptTreeMock, addDeptMock, editDeptMock, deleteDeptMock } = vi.hoisted(() => ({
  deptTreeMock: vi.fn(),
  addDeptMock: vi.fn(),
  editDeptMock: vi.fn(),
  deleteDeptMock: vi.fn(),
}))
vi.mock('../../api/iam.js', () => ({
  deptTree: deptTreeMock,
  addDept: addDeptMock,
  editDept: editDeptMock,
  deleteDept: deleteDeptMock,
}))

import DeptPage from './DeptPage.jsx'
import { setPermissions } from '../../utils/perm.js'

// 部门树 fake：两层（1 为父含子级，2 为叶子）——id/parentId 为字符串（后端序列化口径）
const FAKE_TREE = [
  {
    id: '1',
    parentId: '0',
    deptName: '物流科',
    sort: 1,
    status: 'ENABLED',
    children: [
      { id: '2', parentId: '1', deptName: '成型班组', sort: 1, status: 'ENABLED', children: [] },
    ],
  },
]

const OK_TREE = { code: 200, data: FAKE_TREE, message: '成功' }

const ALL_PERMS = [
  'system:dept:list',
  'system:dept:add',
  'system:dept:edit',
  'system:dept:remove',
]

/** 渲染包一层 antd <App>——App.useApp() 在 <App> 之外是空对象，message 调用会抛 TypeError */
function renderPage() {
  return render(
    <App>
      <DeptPage />
    </App>,
  )
}

describe('DeptPage', () => {
  beforeEach(() => {
    setPermissions(ALL_PERMS)
    deptTreeMock.mockReset().mockResolvedValue(OK_TREE)
    addDeptMock.mockReset().mockResolvedValue({ code: 200, data: 9, message: '成功' })
    editDeptMock.mockReset().mockResolvedValue({ code: 200, data: null, message: '成功' })
    deleteDeptMock.mockReset().mockResolvedValue({ code: 200, data: null, message: '成功' })
  })

  it('① 渲染部门树（物流科 → 成型班组，受控展开）', async () => {
    renderPage()
    await waitFor(() => expect(deptTreeMock).toHaveBeenCalled())
    await waitFor(() => expect(screen.getByText('物流科')).toBeInTheDocument())
    expect(screen.getByText('成型班组')).toBeInTheDocument()
  })

  it('② 无 system:dept:list 时页面可挂载不崩', async () => {
    setPermissions([])
    expect(() => renderPage()).not.toThrow()
    await waitFor(() => expect(screen.getByText('物流科')).toBeInTheDocument())
  })

  it('③ 新增部门：填名称保存 → addDept 调用且列表刷新', async () => {
    const user = userEvent.setup()
    renderPage()
    await waitFor(() => expect(screen.getByText('物流科')).toBeInTheDocument())

    await user.click(screen.getByRole('button', { name: /新增部门/ }))
    const nameInput = await screen.findByPlaceholderText('如：物流科')
    await user.type(nameInput, '测试部门')
    await user.click(screen.getByRole('button', { name: /^保\s*存$/ }))

    await waitFor(() => expect(addDeptMock).toHaveBeenCalled())
    expect(addDeptMock).toHaveBeenCalledWith(expect.objectContaining({ deptName: '测试部门' }))
    // 保存后 reload → deptTree 再次被调用（首次渲染 1 次 + 刷新 1 次）
    await waitFor(() => expect(deptTreeMock.mock.calls.length).toBeGreaterThanOrEqual(2))
  })

  it('④ 编辑部门：回显该行 deptName → 保存 → editDept 调用', async () => {
    const user = userEvent.setup()
    renderPage()
    await waitFor(() => expect(screen.getByText('物流科')).toBeInTheDocument())

    await user.click(screen.getAllByRole('button', { name: /^编\s*辑$/ })[0])
    await screen.findByDisplayValue('物流科')
    await user.click(screen.getByRole('button', { name: /^保\s*存$/ }))

    await waitFor(() => expect(editDeptMock).toHaveBeenCalled())
    expect(editDeptMock).toHaveBeenCalledWith(
      expect.objectContaining({ id: '1', deptName: '物流科' }),
    )
  })

  it('⑤ 删除守卫：有子级零请求；叶子确认后 deleteDept(id)', async () => {
    const user = userEvent.setup()
    renderPage()
    await waitFor(() => expect(screen.getByText('物流科')).toBeInTheDocument())

    const deleteButtons = screen.getAllByRole('button', { name: /^删\s*除$/ })
    expect(deleteButtons).toHaveLength(2) // 物流科（有子级）/ 成型班组（叶子）

    // 有子级：点删除仅弹提示，不发请求
    await user.click(deleteButtons[0])
    expect(deleteDeptMock).not.toHaveBeenCalled()

    // 叶子：Popconfirm 确认后发请求
    await user.click(deleteButtons[1])
    await user.click(await screen.findByRole('button', { name: /^确\s*认$/ }))
    await waitFor(() => expect(deleteDeptMock).toHaveBeenCalledWith('2'))
  })

  it('⑥ 权限显隐：仅 list 时无「新增部门」，补 add 后出现', async () => {
    setPermissions(['system:dept:list'])
    const { unmount } = renderPage()
    await waitFor(() => expect(screen.getByText('物流科')).toBeInTheDocument())
    expect(screen.queryByRole('button', { name: /新增部门/ })).not.toBeInTheDocument()
    unmount()

    setPermissions(['system:dept:list', 'system:dept:add'])
    renderPage()
    await waitFor(() => expect(screen.getByText('物流科')).toBeInTheDocument())
    expect(screen.getByRole('button', { name: /新增部门/ })).toBeInTheDocument()
  })
})
