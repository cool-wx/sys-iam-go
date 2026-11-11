/**
 * 角色管理页（域6 第 2 个业务页）。
 *
 * 契约（RoleController 源码实测 2026-09-28）：
 * - GET /system/role/list?page=&size=&roleName= → data=Role[] + 顶层 total
 * - Role 字段：roleName/roleCode/dataScope(Int)/status(枚举 name: ENABLED|DISABLED)/remark
 * - 写端点：POST add {roleName!,roleCode!,dataScope?,remark?} / PUT edit {id!,...} /
 *   DELETE delete/{id}
 * - 菜单分配：GET menus/{id} → 已分配菜单 ID 列表；POST menus body {id!, menuIds!}
 *   （分配端点权限 = system:role:edit，源码 ROLE_EDIT——plan 中 system:role:assign 后端不存在）
 * - 回显口径：antd Tree 级联模式只回显叶子 key（父态自动推导）；提交 = 勾选 + 半选合集
 */
import { useCallback, useRef, useState } from 'react'
import { App, Button, Drawer, Form, Input, InputNumber, Modal, Popconfirm, Space, Switch, Tag, Tree } from 'antd'
import { ApartmentOutlined, PlusOutlined } from '@ant-design/icons'
import CrudTable from '../../components/CrudTable.jsx'
import PermButton from '../../components/PermButton.jsx'
import {
  addRole,
  assignRoleMenus,
  deleteRole,
  editRole,
  getRoleMenus,
  listRoles,
  menuTree,
} from '../../api/iam.js'

const PERM = {
  list: 'system:role:list',
  add: 'system:role:add',
  edit: 'system:role:edit', // 编辑与菜单分配同权限（后端 RoleController 均挂 ROLE_EDIT）
  remove: 'system:role:remove',
}

function StatusTag({ value }) {
  return value === 'ENABLED' ? (
    <Tag color="success">正常</Tag>
  ) : (
    <Tag color="error">禁用</Tag>
  )
}

/** Menu 树 → antd Tree treeData（title=菜单名，key=id，保留 id 供祖先推导） */
function toTreeData(nodes) {
  return (nodes || []).map((n) => ({
    title: n.name,
    key: n.id,
    id: n.id,
    children: n.children?.length ? toTreeData(n.children) : undefined,
  }))
}

/** 收集树中作为父出现的 id（有子级的节点） */
function collectParentIds(nodes, into = new Set()) {
  ;(nodes || []).forEach((n) => {
    if (n.children?.length) {
      into.add(n.id)
      collectParentIds(n.children, into)
    }
  })
  return into
}

/**
 * 收集勾选集的全部祖先 id（提交集口径：勾选节点 + 全部祖先，父级含半选与全选两种形态）。
 * 级联语义下勾选节点的每个祖先必然处于全选或半选态——祖先推导与 onCheck 的
 * halfCheckedKeys 等价，但不依赖「用户动过树」（回显后直接保存也不丢父节点）。
 */
function collectAncestors(nodes, checkedSet, into = new Set()) {
  ;(nodes || []).forEach((n) => {
    if (n.children?.length) {
      collectAncestors(n.children, checkedSet, into)
      if (n.children.some((c) => checkedSet.has(c.id) || into.has(c.id))) {
        into.add(n.id)
      }
    }
  })
  return into
}

export default function RolePage() {
  const { message } = App.useApp()
  const tableRef = useRef(null)
  const [modalOpen, setModalOpen] = useState(false)
  const [editing, setEditing] = useState(null) // null=新增，否则 Role 行
  const [submitting, setSubmitting] = useState(false)
  const [form] = Form.useForm()

  // —— 分配菜单抽屉状态 ——
  const [assignRole, setAssignRole] = useState(null) // 当前分配的角色行
  const [treeData, setTreeData] = useState([])
  const [treeLoading, setTreeLoading] = useState(false)
  const [assignSaving, setAssignSaving] = useState(false)
  const [checkedKeys, setCheckedKeys] = useState([]) // 受控勾选（叶子口径，父态级联推导）
  const [expandedKeys, setExpandedKeys] = useState([]) // 受控展开（treeData 异步到位后 defaultExpandAll 已过时机）

  const fetchPage = useCallback((page, size) => listRoles({ page, size }), [])

  const openCreate = () => {
    setEditing(null)
    form.resetFields()
    setModalOpen(true)
  }

  const openEdit = (row) => {
    setEditing(row)
    form.resetFields()
    form.setFieldsValue({
      roleName: row.roleName,
      roleCode: row.roleCode,
      dataScope: row.dataScope ?? undefined,
      remark: row.remark,
      builtin: !!row.builtin,
    })
    setModalOpen(true)
  }

  const submitModal = async () => {
    const values = await form.validateFields()
    setSubmitting(true)
    try {
      if (editing) {
        await editRole({ id: editing.id, ...values })
        message.success('修改角色成功')
      } else {
        await addRole(values)
        message.success('新增角色成功')
      }
      setModalOpen(false)
      tableRef.current?.refresh()
    } catch {
      // 错误提示已由 http 拦截器统一弹出（表单校验错误不进此 catch）
    } finally {
      setSubmitting(false)
    }
  }

  const onDelete = async (row) => {
    // 体验层守卫（权威拦截在后端 IllegalArgumentException → 400）
    if (row.builtin) {
      message.warning('系统内置角色不可删除')
      return
    }
    await deleteRole(row.id)
    message.success('删除角色成功')
    tableRef.current?.refresh()
  }

  /** 打开分配抽屉：并行拉菜单树 + 角色已分配 ID（回显只勾叶子，父态由级联推导） */
  const openAssign = async (row) => {
    setAssignRole(row)
    setTreeLoading(true)
    setCheckedKeys([])
    try {
      const [treeR, menuIdsR] = await Promise.all([menuTree(), getRoleMenus(row.id)])
      const tree = Array.isArray(treeR?.data) ? treeR.data : []
      setTreeData(toTreeData(tree))
      setExpandedKeys([...collectParentIds(tree)]) // 异步数据默认全展开（受控）
      const assigned = Array.isArray(menuIdsR?.data) ? menuIdsR.data : []
      const parents = collectParentIds(tree)
      setCheckedKeys(assigned.filter((id) => !parents.has(id)))
    } catch {
      // 错误提示已由 http 拦截器统一弹出
    } finally {
      setTreeLoading(false)
    }
  }

  const onTreeCheck = (checked) => {
    setCheckedKeys(Array.isArray(checked) ? checked : checked.checked)
  }

  const saveAssign = async () => {
    if (!assignRole) return
    const checkedSet = new Set(checkedKeys)
    const menuIds = [...checkedSet, ...collectAncestors(treeData, checkedSet)]
    setAssignSaving(true)
    try {
      await assignRoleMenus(assignRole.id, menuIds)
      message.success(`角色「${assignRole.roleName}」菜单分配成功`)
      setAssignRole(null)
      tableRef.current?.refresh()
    } catch {
      // 错误提示已由 http 拦截器统一弹出
    } finally {
      setAssignSaving(false)
    }
  }

  const columns = [
    { title: '角色名称', dataIndex: 'roleName' },
    { title: '角色编码', dataIndex: 'roleCode' },
    {
      title: '状态',
      dataIndex: 'status',
      width: 90,
      render: (v) => <StatusTag value={v} />,
    },
    {
      title: '系统内置',
      dataIndex: 'builtin',
      width: 100,
      render: (v) => (v ? <Tag color="warning">内置</Tag> : '—'),
    },
    { title: '备注', dataIndex: 'remark', ellipsis: true },
    {
      title: '操作',
      key: 'actions',
      width: 240,
      render: (_, row) => (
        <Space size="small" wrap>
          <PermButton perm={PERM.edit} size="small" onClick={() => openEdit(row)}>
            编辑
          </PermButton>
          <PermButton
            perm={PERM.edit}
            size="small"
            icon={<ApartmentOutlined />}
            onClick={() => openAssign(row)}
          >
            分配菜单
          </PermButton>
          {row.builtin ? (
            <PermButton
              perm={PERM.remove}
              size="small"
              danger
              disabled
              title="系统内置角色不可删除"
            >
              删除
            </PermButton>
          ) : (
            <Popconfirm
              title="确认删除该角色？"
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
          新增角色
        </PermButton>
      </Space>

      <CrudTable ref={tableRef} fetchPage={fetchPage} columns={columns} />

      <Modal
        title={editing ? `编辑角色：${editing.roleName}` : '新增角色'}
        open={modalOpen}
        onOk={submitModal}
        onCancel={() => setModalOpen(false)}
        confirmLoading={submitting}
        okText="保存"
        cancelText="取消"
        forceRender
      >
        <Form form={form} layout="vertical" autoComplete="off">
          <Form.Item
            name="roleName"
            label="角色名称"
            rules={[{ required: true, message: '请输入角色名称' }]}
          >
            <Input placeholder="如：管理员" />
          </Form.Item>
          <Form.Item
            name="roleCode"
            label="角色编码"
            rules={[{ required: true, message: '请输入角色编码' }]}
          >
            <Input placeholder="如：admin" />
          </Form.Item>
          <Form.Item name="dataScope" label="数据权限范围">
            <InputNumber min={0} style={{ width: '100%' }} placeholder="选填，默认 1" />
          </Form.Item>
          <Form.Item name="remark" label="备注">
            <Input.TextArea placeholder="选填" rows={2} />
          </Form.Item>
          <Form.Item name="builtin" label="系统内置" valuePropName="checked" initialValue={false}>
            <Switch />
          </Form.Item>
        </Form>
      </Modal>

      <Drawer
        title={`分配菜单：${assignRole?.roleName || ''}`}
        width={420}
        open={!!assignRole}
        onClose={() => setAssignRole(null)}
        footer={
          <Space style={{ float: 'right' }}>
            <Button onClick={() => setAssignRole(null)}>取消</Button>
            <Button type="primary" loading={assignSaving} onClick={saveAssign}>
              保存
            </Button>
          </Space>
        }
      >
        <Tree
          checkable
          motion={false} // 禁展开动画：受控展开即点即开（jsdom 下 rc-motion 折叠动画不结束会吞节点）
          treeData={treeData}
          loading={treeLoading}
          checkedKeys={checkedKeys}
          expandedKeys={expandedKeys}
          onExpand={(keys) => setExpandedKeys(keys)}
          onCheck={onTreeCheck}
        />
      </Drawer>
    </div>
  )
}
