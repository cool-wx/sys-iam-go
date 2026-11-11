// @vitest-environment jsdom
/**
 * 文件管理页组件测试（jsdom）——「系统管理→文件管理」接 /api/v1/files 的五条关键行为：
 * ① 无 system:file:list 权限 → 整页不发起任何请求（分类树端点同挂 file list 档）；
 * ② 有权限 → 加载分类树 + 文件列表并渲染行（文件名/中文可读大小/分页总数）；
 * ③ 选中分类节点 → 列表按 categoryId 重查；
 * ④ 上传成功 → FormData 带选中分类 categoryId（无描述键——Go 上传链路无该参数），且刷新列表；
 * ⑤ 下载点击发起 downloadFile(id) 且文件名取自行 fileName；删除经确认后 deleteFile(id) 并刷新。
 *
 * iam 模块整体 mock（axios 层与拦截器不在此测）；渲染包一层 antd <App>——
 * App.useApp() 在 <App> 之外拿到的是空对象（antd AppContext 默认 {message:{}}），
 * message.success 会抛 TypeError 并吞掉其后的刷新调用，测试将失去判别力。
 */
import '@testing-library/jest-dom/vitest'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { App } from 'antd'

const {
  listFilesMock,
  uploadFileMock,
  downloadFileMock,
  deleteFileMock,
  fileCategoryTreeMock,
  addFileCategoryMock,
  editFileCategoryMock,
  deleteFileCategoryMock,
} = vi.hoisted(() => ({
  listFilesMock: vi.fn(),
  uploadFileMock: vi.fn(),
  downloadFileMock: vi.fn(),
  deleteFileMock: vi.fn(),
  fileCategoryTreeMock: vi.fn(),
  addFileCategoryMock: vi.fn(),
  editFileCategoryMock: vi.fn(),
  deleteFileCategoryMock: vi.fn(),
}))

vi.mock('../../api/iam.js', () => ({
  listFiles: listFilesMock,
  uploadFile: uploadFileMock,
  downloadFile: downloadFileMock,
  deleteFile: deleteFileMock,
  fileCategoryTree: fileCategoryTreeMock,
  addFileCategory: addFileCategoryMock,
  editFileCategory: editFileCategoryMock,
  deleteFileCategory: deleteFileCategoryMock,
}))

import FilePage from './FilePage.jsx'
import { setPermissions } from '../../utils/perm.js'

const ALL_PERMS = [
  'system:file:list',
  'system:file:upload',
  'system:file:download',
  'system:file:remove',
  'system:file:add',
  'system:file:edit',
]

// 分类树 fake：工艺文档(5) 含子节点 图纸(6)
const FAKE_TREE = [{ id: 5, categoryName: '工艺文档', children: [{ id: 6, categoryName: '图纸' }] }]

// 文件列表 fake：行 categoryName 取「归档区」而非树节点名——树与表格若渲染同名文本，
// getByText 会命中多个节点导致断言歧义，取相异名称保证断言唯一。
// 行数据不含 description：Go 上传链路无该参数（FileInfoResp.description 恒 null），
// 页面无「描述」列
const FAKE_FILES = {
  code: 0,
  message: 'ok',
  total: 2,
  data: [
    {
      id: 101,
      fileName: '作业指导书.pdf',
      fileType: 'application/pdf',
      fileSize: 2048,
      categoryId: 5,
      categoryName: '归档区',
    },
    {
      id: 102,
      fileName: '产线大图.png',
      fileType: 'image/png',
      fileSize: 3145728,
      categoryId: 6,
      categoryName: '归档区',
    },
  ],
}

function renderPage() {
  return render(
    <App>
      <FilePage />
    </App>,
  )
}

describe('FilePage', () => {
  beforeEach(() => {
    setPermissions(ALL_PERMS)
    listFilesMock.mockReset().mockResolvedValue(FAKE_FILES)
    fileCategoryTreeMock
      .mockReset()
      .mockResolvedValue({ code: 200, data: FAKE_TREE, message: '成功' })
    uploadFileMock.mockReset().mockResolvedValue({ code: 0, data: {}, message: 'ok' })
    // 下载响应：octet-stream + 正确 body 字节。Go 后端带 RFC 5987 Content-Disposition，
    // 此处故意投喂一个**与行数据不同名**的头：断言文件名唯一取自行 fileName，
    // 实现若回头依赖该头即红。
    downloadFileMock.mockReset().mockResolvedValue({
      data: new Blob(['bytes'], { type: 'application/octet-stream' }),
      headers: { 'content-disposition': 'attachment; filename="后端头名.pdf"' },
    })
    deleteFileMock.mockReset().mockResolvedValue({ code: 200, data: null, message: '成功' })
    addFileCategoryMock.mockReset().mockResolvedValue({ code: 200, data: 9, message: '成功' })
    editFileCategoryMock.mockReset().mockResolvedValue({ code: 200, data: null, message: '成功' })
    deleteFileCategoryMock
      .mockReset()
      .mockResolvedValue({ code: 200, data: null, message: '成功' })
  })

  it('① 无 system:file:list 权限 → 不加载列表与分类树（零请求）', async () => {
    setPermissions([])
    renderPage()

    expect(await screen.findByText(/无文件管理查看权限/)).toBeInTheDocument()
    expect(listFilesMock).not.toHaveBeenCalled()
    expect(fileCategoryTreeMock).not.toHaveBeenCalled()
  })

  it('② 有权限 → 加载分类树 + 文件列表并渲染行（文件名/中文可读大小）', async () => {
    renderPage()

    await waitFor(() => expect(fileCategoryTreeMock).toHaveBeenCalled())
    await waitFor(() => expect(listFilesMock).toHaveBeenCalledWith({ page: 1, size: 20 }))

    // 左树：父节点与子节点均渲染（父子节点 id 收集后默认展开）
    await waitFor(() => expect(screen.getByText('工艺文档')).toBeInTheDocument())
    expect(screen.getByText('图纸')).toBeInTheDocument()

    // 右表：两行 + 字节量中文可读 + 后端 total 分页口径
    await waitFor(() => expect(screen.getByText('作业指导书.pdf')).toBeInTheDocument())
    expect(screen.getByText('产线大图.png')).toBeInTheDocument()
    expect(screen.getByText('2.0 KB')).toBeInTheDocument()
    expect(screen.getByText('3.0 MB')).toBeInTheDocument()
    expect(screen.getByText('共 2 条')).toBeInTheDocument()
  })

  it('③ 选中分类节点 → 列表按 categoryId 重查（页码回到第 1 页）', async () => {
    const user = userEvent.setup()
    renderPage()
    await waitFor(() => expect(screen.getByText('作业指导书.pdf')).toBeInTheDocument())

    await user.click(screen.getByText('图纸'))

    await waitFor(() =>
      expect(listFilesMock).toHaveBeenCalledWith({ categoryId: 6, page: 1, size: 20 }),
    )
  })

  it('④ 上传成功 → FormData 带选中分类（无描述键——Go 上传链路无该参数），且刷新列表', async () => {
    const user = userEvent.setup()
    const { container } = renderPage()
    await waitFor(() => expect(screen.getByText('作业指导书.pdf')).toBeInTheDocument())

    // 先选分类（上传需带 categoryId）
    await user.click(screen.getByText('图纸'))
    const callsBefore = listFilesMock.mock.calls.length

    const input = container.querySelector('input[type="file"]')
    expect(input).toBeTruthy()
    fireEvent.change(input, {
      target: { files: [new File(['txt'], '新图纸.txt', { type: 'text/plain' })] },
    })

    await waitFor(() => expect(uploadFileMock).toHaveBeenCalled())
    const fd = uploadFileMock.mock.calls[0][0]
    expect(fd).toBeInstanceOf(FormData)
    expect(fd.get('categoryId')).toBe('6')
    expect(fd.get('description')).toBeNull()
    expect(screen.queryByPlaceholderText(/上传描述/)).toBeNull()
    expect(fd.get('file')).toBeInstanceOf(File)
    expect(fd.get('file').name).toBe('新图纸.txt')

    await waitFor(() => expect(listFilesMock.mock.calls.length).toBeGreaterThan(callsBefore))
  })

  it('⑤ 下载发起 downloadFile(id) 且文件名取自行 fileName；删除经中文确认后 deleteFile(id) 并刷新', async () => {
    const user = userEvent.setup()
    // blob 下载依赖的浏览器 API（jsdom 未实现 createObjectURL；a.click 会触发导航告警）
    const createObjectURL = vi.fn(() => 'blob:mock')
    global.URL.createObjectURL = createObjectURL
    global.URL.revokeObjectURL = vi.fn()
    // 截获 a.click 的接收者（下载 a 已 append 后即被移除，只能从 this 取回）
    let clickedAnchor = null
    const clickSpy = vi
      .spyOn(HTMLAnchorElement.prototype, 'click')
      .mockImplementation(function () {
        clickedAnchor = this
      })

    try {
      renderPage()
      await waitFor(() => expect(screen.getByText('作业指导书.pdf')).toBeInTheDocument())

      // 下载：第一行（行按钮无图标，antd 两字中文标签插入空格 → 「下 载」）
      await user.click(screen.getAllByRole('button', { name: /^下\s*载$/ })[0])
      await waitFor(() => expect(downloadFileMock).toHaveBeenCalledWith(101))
      expect(createObjectURL).toHaveBeenCalledWith(expect.any(Blob))
      // 文件名唯一来源 = 行数据 fileName（响应无 content-disposition，见 beforeEach 说明）
      await waitFor(() => expect(clickedAnchor?.download).toBe('作业指导书.pdf'))

      // 删除：第二行 → Popconfirm 中文确认后调 deleteFile + 刷新
      const callsBefore = listFilesMock.mock.calls.length
      await user.click(screen.getAllByRole('button', { name: /^删\s*除$/ })[1])
      await user.click(await screen.findByRole('button', { name: /^确\s*认$/ }))

      await waitFor(() => expect(deleteFileMock).toHaveBeenCalledWith(102))
      await waitFor(() => expect(listFilesMock.mock.calls.length).toBeGreaterThan(callsBefore))
    } finally {
      clickSpy.mockRestore()
    }
  })
})
