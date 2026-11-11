// Package tree 泛型树构建器：孤儿提升为根——menu/dept/auth 三域树装配语义的
// 唯一实现点（父 id 为 nil 或 0 归根、父 id 索引未命中提升为根，均由本包承担）。
// menu 与 dept 两域共用本函数（menu/dept service List → Build → TreeResp）。
//
// 语义：
//   - parentIDOf 返回 nil 或 0 → 归入 roots；
//   - parentIDOf 指向 items 中不存在的 id（节点索引 map 未命中）→ 提升为根
//     （孤儿提升）；
//   - 环引用双方 parent 均命中索引 → 均不归根、不可达、静默不产出；
//     无递归父链遍历、仅从根向下装配，天然不死循环（children 分组 map
//     不命中即归 roots 的天然免疫性）；
//   - 子节点顺序与输入序一致（repo 按 sort ASC 供给——输入序即展示序）；
//   - attach 对叶子节点也会以空 children 切片调用一次——回调内借此把 children
//     归一化为非 nil（JSON 出口 [] 而非 null）。
package tree

// Build 构建父子层级树，返回根节点切片（恒非 nil）。
//
// 参数：
//   - items:  扁平节点列表（输入序即子节点展示序）；
//   - idOf:   节点主键提取；
//   - parentIDOf: 父 ID 提取（nil 视为无父）；
//   - attach: 挂接回调——把已完整装配的子树值切片挂进节点（DTO 的 Children
//     字段赋值点）；对叶子节点以空切片调用一次，供归一化空数组。
//
// 实现：一次遍历建节点索引 map[int64]*T + children 分组 map[int64][]int
// （parentIDOf 返回 nil 或 0 或索引未命中 → 归入 roots，单遍收集）；
// 随后仅从根索引后序递归装配——子树先完整组装再经 attach 挂进父节点，
// attach 拿到的是含孙代的值拷贝。环引用节点不在任何根的子树内，永不进入递归。
func Build[T any](items []T, idOf func(T) int64, parentIDOf func(T) *int64, attach func(node *T, children []T)) []T {
	nodes := make([]T, len(items))
	copy(nodes, items)

	index := make(map[int64]*T, len(nodes))
	for i := range nodes {
		index[idOf(nodes[i])] = &nodes[i]
	}

	childrenOf := make(map[int64][]int, len(nodes))
	rootIdx := make([]int, 0, len(nodes))
	for i := range nodes {
		pid := parentIDOf(nodes[i])
		if pid == nil || *pid == 0 {
			rootIdx = append(rootIdx, i)
			continue
		}
		if _, ok := index[*pid]; !ok {
			// 孤儿提升为根（map 未命中 → roots.add）
			rootIdx = append(rootIdx, i)
			continue
		}
		childrenOf[*pid] = append(childrenOf[*pid], i)
	}

	// 后序装配：子树完整组装（含孙代）后再挂进父——attach 收到的值拷贝已含全嵌套。
	var assemble func(i int) T
	assemble = func(i int) T {
		node := &nodes[i]
		children := make([]T, 0, len(childrenOf[idOf(nodes[i])]))
		for _, ci := range childrenOf[idOf(nodes[i])] {
			children = append(children, assemble(ci))
		}
		attach(node, children) // 叶子以空切片调用一次——归一化点
		return *node
	}

	roots := make([]T, 0, len(rootIdx))
	for _, ri := range rootIdx {
		roots = append(roots, assemble(ri))
	}
	return roots
}
