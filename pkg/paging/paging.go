// Package paging 定义全库统一的分页横切件：参数钳位与分页 envelope 数据结构
// （等价源 PageHelper 缺省 1/15 语义；分页形状为后续所有列表域的模板）。
package paging

// Parse 钳位分页参数（编译存根：RED 阶段未实现钳位逻辑）。
func Parse(page, size string) (pageOut, sizeOut int) {
	_, _ = page, size
	return 0, 0
}

// PageData 分页 envelope 数据结构（data 内 {items,total,page,size}）。
type PageData[T any] struct {
	Items []T   `json:"items"`
	Total int64 `json:"total"`
	Page  int   `json:"page"`
	Size  int   `json:"size"`
}
