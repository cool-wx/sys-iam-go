package tree

// Build 泛型树构建器：孤儿提升为根——源系统服务层 buildTree 语义的唯一实现点
// （DeptApplicationService.java:89-107 / MenuApplicationService.java:97-115 /
// AuthApplicationService.java:185-203 三处一致的语义；实体静态 buildTree 是孤儿
// 丢弃语义且全仓库零调用的死代码——勿仿，02-RESEARCH 1.4 死代码警告）。
// menu 与 dept 两域共用本函数（menu/dept service List → Build → TreeResp）。
//
// 语义：
//   - parentIDOf 返回 nil 或 0 → 归入 roots（源 `pid == null || pid == 0L`）；
//   - parentIDOf 指向 items 中不存在的 id（map 未命中）→ 提升为根（孤儿提升，
//     DEPT-02 验收语义）；
//   - 环引用双方 parent 均命中索引 → 均不归根、不可达、静默不产出（无递归父链
//     遍历，天然不死循环，T-2-09）；
//   - 子节点顺序与输入序一致（repo 按 sort ASC 供给）；
//   - attach 对叶子节点也会以空 children 切片调用一次——回调内可借此把 children
//     归一化为非 nil（JSON 出口 [] 而非 null）。
//
// TDD RED stub：故意返回 nil——本包全部行为断言在此实现下必须失败。
func Build[T any](items []T, idOf func(T) int64, parentIDOf func(T) *int64, attach func(node *T, children []T)) []T {
	_, _, _, _ = items, idOf, parentIDOf, attach
	return nil
}
