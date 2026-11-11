/**
 * CrudTable 组件测试（jsdom）：
 * ① fake fetchPage 返回 {data:[{id:1}],total:...} → 断言行渲染与 total 分页器（共 N 条）；
 * ② 点击第 2 页 → fetchPage(2, 20) 参数传递。
 */
import '@testing-library/jest-dom/vitest'
import { describe, it, expect, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import CrudTable from './CrudTable.jsx'

const columns = [{ title: '名称', dataIndex: 'name' }]

function makeFetchPage() {
  return vi.fn(async (page) =>
    page === 1
      ? { data: [{ id: 1, name: '甲' }, { id: 2, name: '乙' }], total: 40 }
      : { data: [{ id: 21, name: '页二行' }], total: 40 },
  )
}

describe('CrudTable', () => {
  it('① 首屏 fetchPage(1,20) → 行渲染 + total 分页器', async () => {
    const fetchPage = makeFetchPage()
    render(<CrudTable fetchPage={fetchPage} columns={columns} />)

    await waitFor(() => expect(screen.getByText('甲')).toBeInTheDocument())
    expect(screen.getByText('乙')).toBeInTheDocument()
    expect(screen.getByText('共 40 条')).toBeInTheDocument()
    expect(fetchPage).toHaveBeenCalledWith(1, 20, undefined)
  })

  it('② 翻页触发 fetchPage(2,20) 并渲染第二页数据', async () => {
    const user = userEvent.setup()
    const fetchPage = makeFetchPage()
    const { container } = render(<CrudTable fetchPage={fetchPage} columns={columns} />)

    await waitFor(() => expect(screen.getByText('甲')).toBeInTheDocument())
    const pageTwo = container.querySelector('.ant-pagination-item-2')
    expect(pageTwo).not.toBeNull()
    await user.click(pageTwo)

    await waitFor(() => expect(fetchPage).toHaveBeenCalledWith(2, 20, undefined))
    await waitFor(() => expect(screen.getByText('页二行')).toBeInTheDocument())
  })
})
