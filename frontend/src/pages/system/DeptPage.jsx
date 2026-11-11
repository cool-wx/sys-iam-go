/**
 * 组织管理页（系统管理组，/system/depts）。
 *
 * 契约（DeptController 源码实测 2026-10-14）：
 * - GET /system/dept/tree → data=Dept[]（自引用 children 全量树，无分页）
 * - Dept 字段：parentId/parentName/deptName/sort/status(UsageStatus: ENABLED|DISABLED)
 * - **id/parentId 序列化为字符串**（雪花 id 直接转 Number 会丢精度——保持原值不作转换）
 * - status 序列化为枚举 name 字符串（'ENABLED'/'DISABLED'）
 * - 写端点：POST add / PUT edit（body 均为 Dept 实体）/ DELETE delete/{id}
 * - 删除守卫：后端 removeById 无子级校验 → 前端对含 children 的节点拦截提示，防孤儿外键
 * - parentId 为 '0'（根）时 TreeSelect 回填 undefined（无「0」节点）；缺省即落根级
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
  Table,
  Tag,
  TreeSelect,
} from 'antd'
import { PlusOutlined, ReloadOutlined } from '@ant-design/icons'
import PermButton from '../../components/PermButton.jsx'
import { addDept, deleteDept, deptTree, editDept } from '../../api/iam.js'

const PERM = {
  list: 'system:dept:list',
  add: 'system:dept:add',
  edit: 'system:dept:edit',
  remove: 'system:dept:remove',
}

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

/** Dept 树 → TreeSelect options（excludeId：编辑时把自身从可选父级中剔除，轻量防环） */
function toSelectData(nodes, excludeId) {
  return (nodes || [])
    .filter((n) => n.id !== excludeId)
    .map((n) => ({
      value: n.id,
      title: n.deptName,
      children: n.children?.length ? toSelectData(n.children, excludeId) : undefined,
    }))
}

/** 收集有子级的节点 id（表格受控展开用，异步数据后 defaultExpandAllRows 已过时机） */
function collectExpandableIds(nodes, into = []) {
  ;(nodes || []).forEach((n) => {
    if (n.children?.length) {
      into.push(n.id)
      collectExpandableIds(n.children, into)
    }
  })
  return into
}

export default function DeptPage() {
  const { message } = App.useApp()
  const [tree, setTree] = useState([])
  const [expandedKeys, setExpandedKeys] = useState([])
  const [loading, setLoading] = useState(false)
  const [modalOpen, setModalOpen] = useState(false)
  const [editing, setEditing] = useState(null) // null=新增，否则 Dept 行
  const [submitting, setSubmitting] = useState(false)
  const [form] = Form.useForm()

  const reload = useCallback(async () => {
    setLoading(true)
    try {
      const r = await deptTree()
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
      // 根级（parentId '0'）回填 undefined——TreeSelect 无「0」节点，否则父级显示空值
      parentId: row.parentId && row.parentId !== '0' ? row.parentId : undefined,
      deptName: row.deptName,
      sort: row.sort,
      status: row.status,
    })
    setModalOpen(true)
  }

  const submitModal = async () => {
    const values = await form.validateFields()
    setSubmitting(true)
    try {
      if (editing) {
        await editDept({ id: editing.id, ...values })
        message.success('修改部门成功')
      } else {
        await addDept(values)
        message.success('新增部门成功')
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
    await deleteDept(row.id)
    message.success('删除部门成功')
    reload()
  }

  const columns = [
    { title: '部门名称', dataIndex: 'deptName' },
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
            // 有子级：后端 remove 无子级校验，前端拦截提示且不发请求（防孤儿外键）
            <PermButton
              perm={PERM.remove}
              size="small"
              danger
              onClick={() => message.warning('该部门存在下级，须先删除下级')}
            >
              删除
            </PermButton>
          ) : (
            <Popconfirm
              title="确认删除该部门？"
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
          新增部门
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
        title={editing ? `编辑部门：${editing.deptName || ''}` : '新增部门'}
        open={modalOpen}
        onOk={submitModal}
        onCancel={() => setModalOpen(false)}
        confirmLoading={submitting}
        okText="保存"
        cancelText="取消"
        forceRender
      >
        <Form form={form} layout="vertical" autoComplete="off">
          <Form.Item name="parentId" label="上级部门">
            <TreeSelect
              treeData={toSelectData(tree, editing?.id)}
              treeDefaultExpandAll
              allowClear
              placeholder="不选则为根级"
            />
          </Form.Item>
          <Form.Item
            name="deptName"
            label="部门名称"
            rules={[{ required: true, message: '请输入部门名称' }]}
          >
            <Input placeholder="如：物流科" />
          </Form.Item>
          <Form.Item name="sort" label="排序">
            <InputNumber min={0} style={{ width: '100%' }} placeholder="数字越小越靠前" />
          </Form.Item>
          <Form.Item name="status" label="状态" initialValue="ENABLED">
            <Select options={STATUS_OPTIONS} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}
