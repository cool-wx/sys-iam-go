package paging

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseClamp 分页参数钳位五断言（P15，源 PageHelper 语义）：
// 缺省 1/15、<1 钳位、size 上限 100、非法输入兜底、合法值原样。
func TestParseClamp(t *testing.T) {
	cases := []struct {
		name     string
		page     string
		size     string
		wantPage int
		wantSize int
	}{
		{"缺省空参", "", "", 1, 15},
		{"零与负", "0", "-1", 1, 15},
		{"size超上限", "2", "200", 2, 100},
		{"非法输入", "abc", "x", 1, 15},
		{"合法原样", "3", "20", 3, 20},
		// WR-03 回归：int64 极值/超上限页码必须钳位 maxPage——否则 (page-1)*size
		// 有符号溢出为负，GORM 静默省略 OFFSET 子句，静默回退第 1 页数据
		{"int64极值页码", "9223372036854775807", "20", maxPage, 20},
		{"页码刚超上限", "1000001", "15", maxPage, 15},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, s := Parse(tc.page, tc.size)
			assert.Equal(t, tc.wantPage, p, "page 输入=%q", tc.page)
			assert.Equal(t, tc.wantSize, s, "size 输入=%q", tc.size)
		})
	}
}

// TestPageDataEmptyItemsMarshal Items 为 nil 时序列化必须输出 [] 而非 null
// （ENG-01 空输入边界定案：空列表输出空数组）。
func TestPageDataEmptyItemsMarshal(t *testing.T) {
	p := PageData[string]{Items: nil, Total: 0, Page: 99, Size: 15}
	bs, err := json.Marshal(p)
	require.NoError(t, err)
	assert.Contains(t, string(bs), `"items":[]`, "空列表应序列化为 [] 而非 null: %s", bs)
	assert.NotContains(t, string(bs), "null")

	// 非空 Items 正常序列化
	p2 := PageData[string]{Items: []string{"a", "b"}, Total: 2, Page: 1, Size: 15}
	bs2, err := json.Marshal(p2)
	require.NoError(t, err)
	assert.Contains(t, string(bs2), `"items":["a","b"]`)
	assert.Contains(t, string(bs2), `"total":2`)
	assert.Contains(t, string(bs2), `"page":1`)
	assert.Contains(t, string(bs2), `"size":15`)
}
