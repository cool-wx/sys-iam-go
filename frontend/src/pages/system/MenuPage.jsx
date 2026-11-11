/**
 * 菜单管理页（域6 第 3 个业务页）。
 *
 * 契约（MenuController 源码实测 2026-09-28）：
 * - GET /system/menu/tree → data=Menu[]（自引用 children 全量树，无分页）
 * - Menu 字段：parentId/parentName/name/type(MenuType: DIR|MENU|BUTTON，序列化为 name)/
 *   path/component/permission/icon/sort/visible/status(ENABLED|DISABLED)
 * - 写端点：POST add / PUT edit（body 均为 Menu 实体）/ DELETE delete/{id}
 * - 枚举口径与 UserPage 实测一致：序列化/反序列化均走 name 字符串（'DIR'/'ENABLED'）
 * - 删除守卫：有子级须先删子级（前端拦截提示，后端亦有约束）
 */
import { useCallback, useEffect, useState } from 'react'
import {
  App,
  Button,
  Form,
  Input,
  InputNumber,
  Modal,
  Popconfirm,
  Select,
  Space,
  Switch,
  Table,
  Tag,
  TreeSelect,
} from 'antd'
import { PlusOutlined, ReloadOutlined } from '@ant-design/icons'
import PermButton from '../../components/PermButton.jsx'
import { addMenu, deleteMenu, editMenu, menuTree } from '../../api/iam.js'

const PERM = {
  list: 'system:menu:list',
  add: 'system:menu:add',
  edit: 'system:menu:edit',
  remove: 'system:menu:remove',
}

const TYPE_INFO = { DIR: '目录', MENU: '菜单', BUTTON: '按钮' }
const TYPE_OPTIONS = Object.entries(TYPE_INFO).map(([value, label]) => ({ value, label }))
const STATUS_OPTIONS = [
  { value: 'ENABLED', label: '正常' },
  { value: 'DISABLED', label: '禁用' },
]

function StatusTag({ value }) {
  return value === 'ENABLED' ? (
    <Tag color="success">正常</Tag>
  ) : (
    <Tag color="error">禁用</Tag>
  )
}

/** Menu 树 → TreeSelect options（excludeId：编辑时把自身从可选父级中剔除，轻量防环） */
function toSelectData(nodes, excludeId) {
  return (nodes || [])
    .filter((n) => n.id !== excludeId)
    .map((n) => ({
      value: n.id,
      title: n.name,
      children: n.children?.length ? toSelectData(n.children, excludeId) : undefined,
    }))
}

/** 收集有子级的节点 id（表格受控展开用） */
function collectExpandableIds(nodes, into = []) {
  ;(nodes || []).forEach((n) => {
    if (n.children?.length) {
      into.push(n.id)
      collectExpandableIds(n.children, into)
    }
  })
  return into
}

export default function MenuPage() {
  const { message } = App.useApp()
  const [tree, setTree] = useState([])
  const [expandedKeys, setExpandedKeys] = useState([]) // 受控展开（异步数据后 defaultExpandAllRows 已过时机）
  const [loading, setLoading] = useState(false)
  const [modalOpen, setModalOpen] = useState(false)
  const [editing, setEditing] = useState(null) // null=新增，否则 Menu 行
  const [submitting, setSubmitting] = useState(false)
  const [form] = Form.useForm()

  const reload = useCallback(async () => {
    setLoading(true)
    try {
      const r = await menuTree()
      const rows = Array.isArray(r?.data) ? r.data : []
      setTree(rows)
      setExpandedKeys(collectExpandableIds(rows)) // 默认全展开（受控）
    } catch {
      // 错误提示已由 http 拦截器统一弹出
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    reload()
  }, [reload])

  const openCreate = () => {
    setEditing(null)
    form.resetFields()
    setModalOpen(true)
  }

  const openEdit = (row) => {
    setEditing(row)
    form.resetFields()
    form.setFieldsValue({
      parentId: row.parentId ?? undefined,
      name: row.name,
      type: row.type,
      path: row.path,
      component: row.component,
      permission: row.permission,
      icon: row.icon,
      sort: row.sort,
      visible: row.visible,
      status: row.status,
    })
    setModalOpen(true)
  }

  const submitModal = async () => {
    const values = await form.validateFields()
    setSubmitting(true)
    try {
      if (editing) {
        await editMenu({ id: editing.id, ...values })
        message.success('修改菜单成功')
      } else {
        await addMenu(values)
        message.success('新增菜单成功')
      }
      setModalOpen(false)
      reload()
    } catch {
      // 错误提示已由 http 拦截器统一弹出（表单校验错误不进此 catch）
    } finally {
      setSubmitting(false)
    }
  }

  const onDelete = async (row) => {
    await deleteMenu(row.id)
    message.success('删除菜单成功')
    reload()
  }

  const columns = [
    { title: '菜单名称', dataIndex: 'name' },
    {
      title: '类型',
      dataIndex: 'type',
      width: 90,
      render: (v) => TYPE_INFO[v] || v,
    },
    { title: '路由路径', dataIndex: 'path' },
    { title: '权限标识', dataIndex: 'permission' },
    { title: '排序', dataIndex: 'sort', width: 80 },
    {
      title: '状态',
      dataIndex: 'status',
      width: 90,
      render: (v) => <StatusTag value={v} />,
    },
    {
      title: '操作',
      key: 'actions',
      width: 160,
      render: (_, row) => (
        <Space size="small" wrap>
          <PermButton perm={PERM.edit} size="small" onClick={() => openEdit(row)}>
            编辑
          </PermButton>
          {row.children?.length ? (
            <PermButton
              perm={PERM.remove}
              size="small"
              danger
              onClick={() => message.warning('该菜单存在子级，须先删除子级')}
            >
              删除
            </PermButton>
          ) : (
            <Popconfirm
              title="确认删除该菜单？"
              description="删除后不可恢复"
              onConfirm={() => onDelete(row)}
              okText="确认"
              cancelText="取消"
            >
              <PermButton perm={PERM.remove} size="small" danger>
                删除
              </PermButton>
            </Popconfirm>
          )}
        </Space>
      ),
    },
  ]

  return (
    <div>
      <Space style={{ marginBottom: 12 }}>
        <PermButton perm={PERM.add} type="primary" icon={<PlusOutlined />} onClick={openCreate}>
          新增菜单
        </PermButton>
        <Button icon={<ReloadOutlined />} onClick={reload}>
          刷新
        </Button>
      </Space>

      <Table
        rowKey="id"
        columns={columns}
        dataSource={tree}
        loading={loading}
        pagination={false}
        expandedRowKeys={expandedKeys}
        onExpandedRowsChange={setExpandedKeys}
        locale={{ emptyText: '暂无数据' }}
      />

      <Modal
        title={editing ? `编辑菜单：${editing.name || ''}` : '新增菜单'}
        open={modalOpen}
        onOk={submitModal}
        onCancel={() => setModalOpen(false)}
        confirmLoading={submitting}
        okText="保存"
        cancelText="取消"
        forceRender
      >
        <Form form={form} layout="vertical" autoComplete="off">
          <Form.Item name="parentId" label="父级菜单">
            <TreeSelect
              treeData={toSelectData(tree, editing?.id)}
              treeDefaultExpandAll
              allowClear
              placeholder="不选则为根级"
            />
          </Form.Item>
          <Form.Item
            name="name"
            label="菜单名称"
            rules={[{ required: true, message: '请输入菜单名称' }]}
          >
            <Input placeholder="如：系统管理" />
          </Form.Item>
          <Form.Item name="type" label="类型" rules={[{ required: true, message: '请选择类型' }]}>
            <Select options={TYPE_OPTIONS} placeholder="目录 / 菜单 / 按钮" />
          </Form.Item>
          <Form.Item name="path" label="路由路径">
            <Input placeholder="如 /system/users" />
          </Form.Item>
          <Form.Item name="component" label="组件路径">
            <Input placeholder="选填" />
          </Form.Item>
          <Form.Item name="permission" label="权限标识">
            <Input placeholder="如 system:user:list" />
          </Form.Item>
          <Form.Item name="icon" label="图标">
            <Input placeholder="选填" />
          </Form.Item>
          <Form.Item name="sort" label="排序">
            <InputNumber min={0} style={{ width: '100%' }} placeholder="数字越小越靠前" />
          </Form.Item>
          <Form.Item name="visible" label="是否可见" valuePropName="checked" initialValue={true}>
            <Switch checkedChildren="可见" unCheckedChildren="隐藏" />
          </Form.Item>
          <Form.Item name="status" label="状态" initialValue="ENABLED">
            <Select options={STATUS_OPTIONS} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}
