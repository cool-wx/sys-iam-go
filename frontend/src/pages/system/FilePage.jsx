/**
 * 文件管理页——接 Go 后端 /api/v1/files 与 /api/v1/file-categories（router.go 路由面）。
 *
 * 契约（internal/handler/file.go 与 internal/model/dto/file.go 实测）：
 * - GET /files?category_id=&page=&size= → data={items,total,...}（分页信封，
 *   api 层归一为 {data, total}；query 参数名 snake_case，页面状态变量仍名 categoryId）
 *   FileInfo 字段：categoryId/categoryName/fileName/fileType/fileSize(字节)；
 *   后端上传链路无 description 参数（FileInfoResp.description 恒 null），本页不设描述输入与列
 * - POST /files/upload multipart（file 必填 + category_id?）→ data=FileInfo
 *   （api 层把 FormData 的 categoryId 键改名 category_id 后透传——handler PostForm("category_id")）
 * - GET /files/download/:id → octet-stream 附件（无信封；http 拦截器按 responseType:'blob'
 *   旁路解包，返回完整 resp——读 resp.data 为 blob 即可；响应带 Content-Disposition
 *   （RFC 5987），但页面文件名仍取行数据 fileName，不依赖响应头）
 * - DELETE /files/:id
 * - 分类：GET tree（自引用 children）/ POST / PUT（实体直传）/ DELETE :id
 * - 权限标识与后端 RequirePerm 常量逐字对齐（perms.go FILE_*）；
 *   分类树端点同挂 file list 档，故无 list 权限时整页不发起任何请求
 *
 * 判据一致性：Go 后端不做文件类型/大小白名单，前端**不得**自行加限制
 * ——前后端判据不一致本身就是缺陷类。此处仅在 beforeUpload 拦空文件。
 */
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  App,
  Card,
  Col,
  Empty,
  Form,
  Input,
  InputNumber,
  Modal,
  Popconfirm,
  Row,
  Space,
  Tree,
  TreeSelect,
  Typography,
  Upload,
} from 'antd'
import { FolderAddOutlined, UploadOutlined } from '@ant-design/icons'
import CrudTable from '../../components/CrudTable.jsx'
import PermButton from '../../components/PermButton.jsx'
import { hasPerm } from '../../utils/perm.js'
import {
  addFileCategory,
  deleteFile,
  deleteFileCategory,
  downloadFile,
  editFileCategory,
  fileCategoryTree,
  listFiles,
  uploadFile,
} from '../../api/iam.js'

const PERM = {
  list: 'system:file:list',
  upload: 'system:file:upload',
  download: 'system:file:download',
  remove: 'system:file:remove',
  add: 'system:file:add',
  edit: 'system:file:edit',
}

/** 字节量中文可读（fileSize 为权威字段，单位字节） */
function formatSize(bytes) {
  if (!Number.isFinite(bytes) || bytes < 0) return '-'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`
}

/** 以原始文件名触发浏览器下载（临时 a 标签，objectURL 用后即撤） */
function triggerBlobDownload(blob, fileName) {
  const url = window.URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = fileName
  document.body.appendChild(a)
  a.click()
  a.remove()
  window.URL.revokeObjectURL(url)
}

/** FileCategory 树 → antd Tree treeData（title=分类名，key=id，保留 id 供祖先收集） */
function toTreeData(nodes) {
  return (nodes || []).map((n) => ({
    title: n.categoryName,
    key: n.id,
    id: n.id,
    children: n.children?.length ? toTreeData(n.children) : undefined,
  }))
}

/** 分类树 → TreeSelect 选项；excludeId 剔除自身及其后代（防把分类挂到自己/后代下成环） */
function toTreeSelectData(nodes, excludeId) {
  return (nodes || [])
    .filter((n) => n.id !== excludeId)
    .map((n) => ({
      title: n.categoryName,
      value: n.id,
      children: n.children?.length ? toTreeSelectData(n.children, excludeId) : undefined,
    }))
}

/** 收集树中作为父出现的 id（有子级的节点）——异步 treeData 到位后受控展开用 */
function collectParentIds(nodes, into = new Set()) {
  ;(nodes || []).forEach((n) => {
    if (n.children?.length) {
      into.add(n.id)
      collectParentIds(n.children, into)
    }
  })
  return into
}

/** 分类节点查找（深度优先，未命中返回 null） */
function findCategoryNode(nodes, id) {
  if (id == null) return null
  for (const n of nodes || []) {
    if (n.id === id) return n
    const hit = findCategoryNode(n.children, id)
    if (hit) return hit
  }
  return null
}

export default function FilePage() {
  const { message } = App.useApp()
  const canList = hasPerm(PERM.list)
  const tableRef = useRef(null)

  // —— 左侧分类树 ——
  const [categories, setCategories] = useState([])
  const [categoryId, setCategoryId] = useState(null) // 选中分类；null=不过滤（全部文件）
  const [expandedKeys, setExpandedKeys] = useState([])
  const [treeLoading, setTreeLoading] = useState(false)

  // —— 右侧上传工具条 ——
  const [uploading, setUploading] = useState(false)

  // —— 分类表单弹窗 ——
  const [categoryModalOpen, setCategoryModalOpen] = useState(false)
  const [editingCategory, setEditingCategory] = useState(null) // null=新增，否则 FileCategory 节点
  const [submitting, setSubmitting] = useState(false)
  const [form] = Form.useForm()

  const treeData = useMemo(() => toTreeData(categories), [categories])
  const treeSelectData = useMemo(
    () => toTreeSelectData(categories, editingCategory?.id),
    [categories, editingCategory],
  )
  const selectedCategoryName = findCategoryNode(categories, categoryId)?.categoryName || ''

  const loadTree = useCallback(async () => {
    setTreeLoading(true)
    try {
      const r = await fileCategoryTree()
      const tree = Array.isArray(r?.data) ? r.data : []
      setCategories(tree)
      setExpandedKeys([...collectParentIds(tree)]) // 受控展开：treeData 异步到位后 defaultExpandAll 已过时机
    } catch {
      // 错误提示已由 http 拦截器统一弹出
    } finally {
      setTreeLoading(false)
    }
  }, [])

  useEffect(() => {
    if (!canList) return
    loadTree()
  }, [canList, loadTree])

  // 列表查询：选中分类即带 categoryId，否则不带（axios 会丢弃 undefined 参数）
  const fetchPage = useCallback(
    (page, size) => listFiles(categoryId != null ? { categoryId, page, size } : { page, size }),
    [categoryId],
  )

  // —— 上传 ——
  const beforeUpload = (file) => {
    // 仅拦空文件；类型/大小白名单与后端判据一致地不设
    if (!file || file.size <= 0) {
      message.error('不能上传空文件')
      return Upload.LIST_IGNORE
    }
    return true
  }

  const customRequest = async ({ file, onSuccess, onError }) => {
    setUploading(true)
    try {
      const fd = new FormData()
      fd.append('file', file)
      if (categoryId != null) fd.append('categoryId', String(categoryId))
      await uploadFile(fd)
      message.success('文件上传成功')
      tableRef.current?.refresh()
      onSuccess?.(file)
    } catch (e) {
      onError?.(e)
    } finally {
      setUploading(false)
    }
  }

  // —— 行操作 ——
  const onDownload = async (row) => {
    try {
      const resp = await downloadFile(row.id)
      const blob = resp?.data instanceof Blob ? resp.data : new Blob([resp?.data ?? ''])
      // 文件名取行数据 fileName（Go 下载响应虽带 RFC 5987 Content-Disposition，
      // 页面不解析响应头取名——口径单一、行为与列表所见一致）
      triggerBlobDownload(blob, row.fileName || 'download')
    } catch {
      // 错误提示已由 http 拦截器统一弹出
    }
  }

  const onDelete = async (row) => {
    try {
      await deleteFile(row.id)
      message.success('删除文件成功')
      tableRef.current?.refresh()
    } catch {
      // 错误提示已由 http 拦截器统一弹出
    }
  }

  // —— 分类增删改 ——
  const openCategoryCreate = () => {
    setEditingCategory(null)
    form.resetFields()
    form.setFieldsValue({ parentId: categoryId ?? undefined }) // 默认挂在选中分类下，不选即根分类
    setCategoryModalOpen(true)
  }

  const openCategoryEdit = () => {
    const node = findCategoryNode(categories, categoryId)
    if (!node) return
    setEditingCategory(node)
    form.resetFields()
    form.setFieldsValue({
      parentId: node.parentId || undefined,
      categoryName: node.categoryName,
      categoryType: node.categoryType ?? undefined,
      sort: node.sort ?? undefined,
    })
    setCategoryModalOpen(true)
  }

  const submitCategory = async () => {
    const values = await form.validateFields()
    setSubmitting(true)
    try {
      if (editingCategory) {
        await editFileCategory({ id: editingCategory.id, ...values })
        message.success('修改分类成功')
      } else {
        await addFileCategory(values)
        message.success('新增分类成功')
      }
      setCategoryModalOpen(false)
      await loadTree()
    } catch {
      // 错误提示已由 http 拦截器统一弹出（表单校验错误不进此 catch）
    } finally {
      setSubmitting(false)
    }
  }

  const onDeleteCategory = async () => {
    if (categoryId == null) return
    try {
      await deleteFileCategory(categoryId)
      message.success('删除分类成功')
      setCategoryId(null) // 选中分类已删，回「全部文件」视图
      await loadTree()
    } catch {
      // 错误提示已由 http 拦截器统一弹出
    }
  }

  const columns = [
    { title: '文件名', dataIndex: 'fileName', ellipsis: true },
    { title: '类型', dataIndex: 'fileType', width: 160, render: (v) => v || '-' },
    { title: '大小', dataIndex: 'fileSize', width: 100, render: (v) => formatSize(v) },
    { title: '分类', dataIndex: 'categoryName', width: 160, render: (v) => v || '未分类' },
    {
      title: '操作',
      key: 'actions',
      width: 150,
      render: (_, row) => (
        <Space size="small" wrap>
          <PermButton perm={PERM.download} size="small" onClick={() => onDownload(row)}>
            下载
          </PermButton>
          <Popconfirm
            title="确认删除该文件？"
            description="删除后不可恢复（对象存储中的实体一并删除）"
            onConfirm={() => onDelete(row)}
            okText="确认"
            cancelText="取消"
          >
            <PermButton perm={PERM.remove} size="small" danger>
              删除
            </PermButton>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  // 分类树端点亦挂 FILE_LIST：无 list 权限时整页不渲染、不请求（零请求，避免 401 中文报错刷屏）
  if (!canList) {
    return <Empty description="无文件管理查看权限（system:file:list），请联系管理员分配" />
  }

  return (
    <Row gutter={16}>
      <Col xs={24} md={8} lg={6}>
        <Card
          size="small"
          title="文件分类"
          extra={
            <Typography.Link onClick={() => setCategoryId(null)}>查看全部</Typography.Link>
          }
        >
          <Space size="small" style={{ marginBottom: 12 }} wrap>
            <PermButton
              perm={PERM.add}
              size="small"
              icon={<FolderAddOutlined />}
              onClick={openCategoryCreate}
            >
              新增分类
            </PermButton>
            <PermButton
              perm={PERM.edit}
              size="small"
              disabled={categoryId == null}
              onClick={openCategoryEdit}
            >
              修改分类
            </PermButton>
            <Popconfirm
              title="确认删除该分类？"
              description="仅删除分类本身，其下文件保留但失去分类归属"
              onConfirm={onDeleteCategory}
              okText="确认"
              cancelText="取消"
              disabled={categoryId == null}
            >
              <PermButton perm={PERM.remove} size="small" danger disabled={categoryId == null}>
                删除分类
              </PermButton>
            </Popconfirm>
          </Space>
          <Tree
            motion={false} // 禁展开动画：受控展开即渲染（jsdom 下 rc-motion 动画不结束会吞节点）
            treeData={treeData}
            loading={treeLoading}
            selectedKeys={categoryId == null ? [] : [categoryId]}
            expandedKeys={expandedKeys}
            onExpand={(keys) => setExpandedKeys(keys)}
            // 再点一次已选节点即取消选中（antd 默认行为）→ 回「全部文件」
            onSelect={(keys) => setCategoryId(keys.length ? keys[0] : null)}
          />
        </Card>
      </Col>

      <Col xs={24} md={16} lg={18}>
        <Space style={{ marginBottom: 12 }} wrap>
          <Upload showUploadList={false} beforeUpload={beforeUpload} customRequest={customRequest}>
            <PermButton perm={PERM.upload} type="primary" icon={<UploadOutlined />} loading={uploading}>
              上传文件
            </PermButton>
          </Upload>
          <Typography.Text type="secondary">
            {`上传至：${selectedCategoryName || '未分类'}`}
          </Typography.Text>
        </Space>

        {/* 分类切换即重挂载：重置内部页码，避免停留在越界页 */}
        <CrudTable
          key={categoryId ?? 'all'}
          ref={tableRef}
          fetchPage={fetchPage}
          columns={columns}
          locale={{ emptyText: '暂无文件' }}
        />
      </Col>

      <Modal
        title={editingCategory ? `编辑分类：${editingCategory.categoryName || ''}` : '新增文件分类'}
        open={categoryModalOpen}
        onOk={submitCategory}
        onCancel={() => setCategoryModalOpen(false)}
        confirmLoading={submitting}
        okText="保存"
        cancelText="取消"
        forceRender
      >
        <Form form={form} layout="vertical" autoComplete="off">
          <Form.Item name="parentId" label="上级分类">
            <TreeSelect
              treeData={treeSelectData}
              allowClear
              treeDefaultExpandAll
              placeholder="不选则为根分类"
            />
          </Form.Item>
          <Form.Item
            name="categoryName"
            label="分类名称"
            rules={[{ required: true, message: '请输入分类名称' }]}
          >
            <Input placeholder="如：作业指导书" />
          </Form.Item>
          <Form.Item name="categoryType" label="分类类型">
            <InputNumber min={0} style={{ width: '100%' }} placeholder="选填" />
          </Form.Item>
          <Form.Item name="sort" label="排序">
            <InputNumber min={0} style={{ width: '100%' }} placeholder="选填" />
          </Form.Item>
        </Form>
      </Modal>
    </Row>
  )
}
