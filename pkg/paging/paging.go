// Package paging 定义全库统一的分页横切件：参数钳位与分页 envelope 数据结构
// （等价源 PageHelper 缺省 1/15、<1 钳位语义；分页形状为后续所有列表域模板：
// data 内 {items,total,page,size}，顶层无 total）。
package paging

import (
	"encoding/json"
	"strconv"
)

// 钳位常量：缺省页码 1 / 页大小 15（源 PageHelper 缺省值）；页大小上限 100
// （DoS 防护，配合连接池 MaxOpenConns 兜底）；页码上限 maxPage——极端大
// 页码使 (page-1)*size 有符号溢出为负，GORM 对非正 Offset 静默省略 OFFSET 子句，
// 会静默返回第 1 页数据，故钳位到固定上限（其上自然空页）。
const (
	defaultPage = 1
	defaultSize = 15
	maxSize     = 100
	maxPage     = 1_000_000
)

// Parse 钳位分页参数：缺省/空串/解析失败/<1 → 页码 1、页大小 15；页大小 >100 →
// 100；页码 >maxPage → maxPage（防 (page-1)*size 溢出静默回退第 1 页）。
func Parse(page, size string) (pageOut, sizeOut int) {
	return ParseWithDefaults(page, size, defaultPage, defaultSize)
}

// ParseWithDefaults 特例分页钳位：钳位逻辑与 Parse 完全一致，仅缺省值参数化——
// 供源 DTO 字段默认值 ≠ 1/15 全局值的域使用（唯一调用方 Olog：源 LogQuery 默认
// 1/10 特例）。既有 Parse 全局 1/15 行为零变化（不得改缺省）。
func ParseWithDefaults(page, size string, defPage, defSize int) (pageOut, sizeOut int) {
	pageOut = clamp(page, defPage)
	sizeOut = clamp(size, defSize)
	if sizeOut > maxSize {
		sizeOut = maxSize
	}
	if pageOut > maxPage {
		pageOut = maxPage
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

// MarshalJSON Items 为 nil 时输出空数组 [] 而非 null（空输入边界定案：
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
