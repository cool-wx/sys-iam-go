// Package paging 定义全库统一的分页横切件：参数钳位与分页 envelope 数据结构
// （等价源 PageHelper 缺省 1/15、<1 钳位语义，P15；分页形状为后续所有列表域模板：
// data 内 {items,total,page,size}，顶层无 total，Anti-Pattern 5）。
package paging

import (
	"encoding/json"
	"strconv"
)

// 钳位常量：缺省页码 1 / 页大小 15（源 PageHelper 缺省值）；页大小上限 100
// （T-1-04 DoS 防护，配合连接池 MaxOpenConns 兜底）；页码无上限（超界自然空页）。
const (
	defaultPage = 1
	defaultSize = 15
	maxSize     = 100
)

// Parse 钳位分页参数：缺省/空串/解析失败/<1 → 页码 1、页大小 15；页大小 >100 → 100。
func Parse(page, size string) (pageOut, sizeOut int) {
	pageOut = clamp(page, defaultPage)
	sizeOut = clamp(size, defaultSize)
	if sizeOut > maxSize {
		sizeOut = maxSize
	}
	return pageOut, sizeOut
}

// clamp 解析非负整数参数：空串/非法/小于 1 → 默认值（源 PageHelper 自动纠偏语义）。
func clamp(s string, dft int) int {
	if s == "" {
		return dft
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < 1 {
		return dft
	}
	return v
}

// PageData 分页 envelope 数据结构。
type PageData[T any] struct {
	Items []T   `json:"items"`
	Total int64 `json:"total"`
	Page  int   `json:"page"`
	Size  int   `json:"size"`
}

// MarshalJSON Items 为 nil 时输出空数组 [] 而非 null（ENG-01 空输入边界定案：
// 空列表恒输出 []，客户端不会拿到 items:null）。在类型序列化出口统一保证，
// 使用方无需各自判空。
func (p PageData[T]) MarshalJSON() ([]byte, error) {
	items := p.Items
	if items == nil {
		items = make([]T, 0)
	}
	return json.Marshal(struct {
		Items []T   `json:"items"`
		Total int64 `json:"total"`
		Page  int   `json:"page"`
		Size  int   `json:"size"`
	}{Items: items, Total: p.Total, Page: p.Page, Size: p.Size})
}
