/**
 * 分页表格封装（后端统一口径：?page=&size= → R{data: records[], total}，260908 全仓统一）。
 *
 * props:
 * - fetchPage: async (page, size, params?) => {data, total}（R 已由 http 拦截器解包）
 * - columns: antd Table columns
 * - params?: 额外查询参数（引用变化即重查）
 * - 其余 props（如 expandable/scroll/size）透传给 antd Table
 * - ref: { refresh() }（React 19 ref-as-prop）——写操作成功后父组件调用刷新
 */
import { useCallback, useEffect, useImperativeHandle, useState } from 'react'
import { Table } from 'antd'

export default function CrudTable({
  fetchPage,
  columns,
  params,
  rowKey = 'id',
  defaultPageSize = 20,
  ref,
  ...rest
}) {
  const [rows, setRows] = useState([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [size, setSize] = useState(defaultPageSize)
  const [loading, setLoading] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const r = await fetchPage(page, size, params)
      setRows(Array.isArray(r?.data) ? r.data : [])
      setTotal(Number(r?.total) || 0)
    } catch {
      // 错误提示已由 http 拦截器统一中文弹出，此处保住 loading 复位
    } finally {
      setLoading(false)
    }
  }, [fetchPage, page, size, params])

  useEffect(() => {
    load()
  }, [load])

  // 暴露 refresh()：翻回不重置当前页（写后原地刷新）
  useImperativeHandle(ref, () => ({ refresh: () => load() }), [load])

  return (
    <Table
      rowKey={rowKey}
      columns={columns}
      dataSource={rows}
      loading={loading}
      locale={{ emptyText: '暂无数据' }}
      pagination={{
        current: page,
        pageSize: size,
        total,
        showSizeChanger: true,
        showTotal: (t) => `共 ${t} 条`,
        onChange: (p, s) => {
          setPage(p)
          setSize(s)
        },
      }}
      {...rest}
    />
  )
}
