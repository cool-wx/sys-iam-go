// Package tree_test 泛型树构建器行为规格（纯逻辑无 Docker）。
// 全部用例直证树装配语义（nil/0 父 id 归根、孤儿提升为根、环不可达静默不产出）。
package tree_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cool-wx/sys-iam-go/internal/tree"
)

// node 测试用树节点：与 dto.MenuTreeResp 同构（值语义 + Children 指针切片）。
// 泛型 Build 对 T 无任何接口约束，测试类型自由。
type node struct {
	ID       int64
	ParentID *int64
	Children []*node
	attached int // attach 回调调用计数（观测挂接行为，含叶子空挂接归一化）
}

// pidOf 取 int64 指针的小工具（构造 parentID）。
func pidOf(v int64) *int64 { return &v }

// attachNode 服务层 attach 回调的测试替身：children 值切片 → 指针切片挂进节点。
// 叶子节点以空切片被调用一次——回调内借此把 Children 归一化为非 nil（JSON 出口 []）。
func attachNode(n *node, children []node) {
	n.attached++
	n.Children = make([]*node, 0, len(children))
	for i := range children {
		c := children[i]
		n.Children = append(n.Children, &c)
	}
}

// build 以测试节点类型驱动 Build（idOf/parentIDOf/attach 三回调的样板封装）。
func build(nodes []node) []node {
	return tree.Build(nodes,
		func(n node) int64 { return n.ID },
		func(n node) *int64 { return n.ParentID },
		attachNode,
	)
}

// TestBuildEmptyListReturnsEmptyNonNil 空列表输入：返回空且非 nil（JSON 出口 []
// 而非 null，空输入边界契约）。
func TestBuildEmptyListReturnsEmptyNonNil(t *testing.T) {
	roots := build(nil)
	require.NotNil(t, roots, "空输入必须返回非 nil 切片")
	assert.Empty(t, roots)

	roots = build([]node{})
	require.NotNil(t, roots, "空切片输入必须返回非 nil 切片")
	assert.Empty(t, roots)
}

// TestBuildTwoGenerations 两代三代正常树：根→子→孙逐级挂接正确，叶子被空挂接
// 一次（Children 归一化点），根节点集合只含真根。
func TestBuildTwoGenerations(t *testing.T) {
	nodes := []node{
		{ID: 1},                     // 根
		{ID: 2, ParentID: pidOf(1)}, // 子
		{ID: 3, ParentID: pidOf(2)}, // 孙
	}
	roots := build(nodes)
	require.Len(t, roots, 1, "仅一个真根")
	root := roots[0]
	assert.Equal(t, int64(1), root.ID)
	assert.Equal(t, 1, root.attached, "根节点应被 attach 一次")
	require.Len(t, root.Children, 1)
	child := root.Children[0]
	assert.Equal(t, int64(2), child.ID)
	assert.Equal(t, 1, child.attached, "子节点应被 attach 一次")
	require.Len(t, child.Children, 1, "孙代应嵌套在子节点下")
	grandchild := child.Children[0]
	assert.Equal(t, int64(3), grandchild.ID)
	assert.Equal(t, 1, grandchild.attached, "叶子节点也应被 attach 一次（空 children 归一化）")
	assert.NotNil(t, grandchild.Children, "叶子 Children 应为非 nil 空切片（JSON [] 非 null）")
	assert.Empty(t, grandchild.Children)
}

// TestBuildNilParentPromotedToRoot parentId=nil：提升为根（nil 即无父，直接归根）。
func TestBuildNilParentPromotedToRoot(t *testing.T) {
	nodes := []node{{ID: 7, ParentID: nil}}
	roots := build(nodes)
	require.Len(t, roots, 1)
	assert.Equal(t, int64(7), roots[0].ID)
}

// TestBuildZeroParentPromotedToRoot parentId=0：提升为根（0 亦按根处理）。
func TestBuildZeroParentPromotedToRoot(t *testing.T) {
	nodes := []node{{ID: 8, ParentID: pidOf(0)}}
	roots := build(nodes)
	require.Len(t, roots, 1)
	assert.Equal(t, int64(8), roots[0].ID)
}

// TestBuildOrphanParentPromotedToRoot parentId 指向不存在节点：提升为根
// （孤儿提升语义直证——父索引未命中即归根，不丢弃）。真根与孤儿同现于根层。
func TestBuildOrphanParentPromotedToRoot(t *testing.T) {
	nodes := []node{
		{ID: 1},                       // 真根
		{ID: 2, ParentID: pidOf(1)},   // 正常子
		{ID: 9, ParentID: pidOf(999)}, // 孤儿：999 不存在
	}
	roots := build(nodes)
	require.Len(t, roots, 2, "真根 + 孤儿均应在根层")
	ids := []int64{roots[0].ID, roots[1].ID}
	assert.ElementsMatch(t, []int64{1, 9}, ids, "孤儿 9 应提升为根（非丢弃）")
	// 正常子仍在根节点的 children 下
	require.Len(t, roots[0].Children, 1)
	assert.Equal(t, int64(2), roots[0].Children[0].ID)
}

// TestBuildCycleDoesNotHang 环引用（A.parent=B、B.parent=A）不死循环：
// Build 必须在有限时间返回（直证——无递归父链遍历，环不可达即终止）。
// 环引用双方 parent 均命中节点索引（非 map 未命中），不满足
// 归根条件（"map 未命中 id → 归入 roots"）：输出树只含真根，环节点静默不产出
// （环即脏数据，被排除在 roots 之外）。
func TestBuildCycleDoesNotHang(t *testing.T) {
	nodes := []node{
		{ID: 1},                       // 真根
		{ID: 10, ParentID: pidOf(11)}, // 环 A
		{ID: 11, ParentID: pidOf(10)}, // 环 B
	}
	done := make(chan []node, 1)
	go func() { done <- build(nodes) }()
	var roots []node
	select {
	case roots = <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Build 在环引用 fixture 上未于 2s 内返回——死循环回归")
	}
	require.Len(t, roots, 1, "环节点双方 parent 均命中索引，不应出现在根层")
	assert.Equal(t, int64(1), roots[0].ID)
}

// TestBuildPreservesChildOrder 子节点顺序与输入序一致（repo 已按 sort ASC 供给——
// 输入序即展示序的契约；repo 按 sort ASC 供给保序收集）。
func TestBuildPreservesChildOrder(t *testing.T) {
	nodes := []node{
		{ID: 1},
		{ID: 21, ParentID: pidOf(1)},
		{ID: 22, ParentID: pidOf(1)},
		{ID: 23, ParentID: pidOf(1)},
	}
	roots := build(nodes)
	require.Len(t, roots, 1)
	require.Len(t, roots[0].Children, 3)
	got := []int64{}
	for _, c := range roots[0].Children {
		got = append(got, c.ID)
	}
	assert.Equal(t, []int64{21, 22, 23}, got, "children 顺序必须与输入序一致")
}
