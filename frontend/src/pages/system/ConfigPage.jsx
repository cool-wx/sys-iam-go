/**
 * 参数配置页（域6 第 5 个业务页）。
 *
 * 契约（ConfigController 源码实测 2026-09-28）：
 * - GET /system/config/list → 全量（无分页；SysConfig 字段 configKey/configValue/configType）
 *   —— 后端无分页端点，CrudTable 以「单页全量」形态包装（total=行数）
 * - 写端点：POST add / PUT edit（实体直传）/ DELETE delete/{id}
 */
import { useCallback, useRef, useState } from 'react'
import { App, Form, Input, Modal, Popconfirm, Space } from 'antd'
import { PlusOutlined } from '@ant-design/icons'
import CrudTable from '../../components/CrudTable.jsx'
import PermButton from '../../components/PermButton.jsx'
import { addConfig, deleteConfig, editConfig, listConfigs } from '../../api/iam.js'

const PERM = {
  add: 'system:config:add',
  edit: 'system:config:edit',
  remove: 'system:config:remove',
}

export default function ConfigPage() {
  const { message } = App.useApp()
  const tableRef = useRef(null)
  const [modalOpen, setModalOpen] = useState(false)
  const [editing, setEditing] = useState(null) // null=新增，否则 SysConfig 行
  const [submitting, setSubmitting] = useState(false)
  const [form] = Form.useForm()

  // 后端 list 全量无分页——包装成 CrudTable 的 {data, total} 单页口径
  const fetchPage = useCallback(async () => {
    const r = await listConfigs()
    const rows = Array.isArray(r?.data) ? r.data : []
    return { data: rows, total: rows.length }
  }, [])

  const openCreate = () => {
    setEditing(null)
    form.resetFields()
    setModalOpen(true)
  }

  const openEdit = (row) => {
    setEditing(row)
    form.resetFields()
    form.setFieldsValue({
      configKey: row.configKey,
      configValue: row.configValue,
      configType: row.configType,
    })
    setModalOpen(true)
  }

  const submitModal = async () => {
    const values = await form.validateFields()
    setSubmitting(true)
    try {
      if (editing) {
        await editConfig({ id: editing.id, ...values })
        message.success('修改参数成功')
      } else {
        await addConfig(values)
        message.success('新增参数成功')
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
    await deleteConfig(row.id)
    message.success('删除参数成功')
    tableRef.current?.refresh()
  }

  const columns = [
    { title: '配置键', dataIndex: 'configKey' },
    { title: '配置值', dataIndex: 'configValue', ellipsis: true },
    { title: '配置类型', dataIndex: 'configType', width: 120 },
    {
      title: '操作',
      key: 'actions',
      width: 140,
      render: (_, row) => (
        <Space size="small" wrap>
          <PermButton perm={PERM.edit} size="small" onClick={() => openEdit(row)}>
            编辑
          </PermButton>
          <Popconfirm
            title="确认删除该参数？"
            description="删除后依赖该参数的功能将回退默认行为"
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

  return (
    <div>
      <Space style={{ marginBottom: 12 }}>
        <PermButton perm={PERM.add} type="primary" icon={<PlusOutlined />} onClick={openCreate}>
          新增参数
        </PermButton>
      </Space>

      <CrudTable ref={tableRef} fetchPage={fetchPage} columns={columns} />

      <Modal
        title={editing ? `编辑参数：${editing.configKey || ''}` : '新增参数'}
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
            name="configKey"
            label="配置键"
            rules={[{ required: true, message: '请输入配置键' }]}
          >
            <Input placeholder="如：site.title" />
          </Form.Item>
          <Form.Item
            name="configValue"
            label="配置值"
            rules={[{ required: true, message: '请输入配置值' }]}
          >
            <Input.TextArea placeholder="参数值" rows={2} />
          </Form.Item>
          <Form.Item name="configType" label="配置类型">
            <Input placeholder="选填" />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}
