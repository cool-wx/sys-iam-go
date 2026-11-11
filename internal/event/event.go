// Package event 进程内名称变更事件机制：
// 轻量 dispatcher（slice of listeners 同步分发，零框架依赖）+ 6 个同步 handler
// （冗余名称列确定性 UPDATE）。发布时序契约：service 层业务写提交成功之后发布，
// handler 于发布者调用栈
// 内同步执行并各自开独立事务——任一 handler 失败 Publish 返回错误、API 500，客户端
// 重试同一请求必重发事件并经 `<> newName` 幂等 UPDATE 自愈（沿双轨同步
// 纪律；发布条件**不设**「新名≠旧名」短路——主行已提交后重试
// 同一请求时旧行已是新名，新≠旧守卫恒假、事件永不重发，自愈路径不可达；各发布域
// 改为「请求显式带名即发布」，幂等 UPDATE 使同名重发退化为廉价 no-op）。
//
// 已知限制：同实体并发改名的多个
// 同步事件之间无顺序保证——主行按提交序落库，但 handler 于各自发布者调用栈内
// 同步执行，执行序不受提交序约束；UPDATE 仅以 `冗余列 <> newName` 为条件、无旧名
// CAS，后到的旧名事件会覆盖先到的新名，冗余列终值可落后于主行直到该实体下一次
// 改名（或任一显式带当前名的请求经幂等重发拉齐——幂等重发的廉价自愈面）。
// 封闭方案（旧名 CAS + NULL 旧名列双形态 / 按实体 ID 进程内串行化）须重新过
// 设计评审，当前单机低并发场景按已知限制接受。
package event

import "context"

// Type 事件类型常量的字符串值。
type Type string

// 5 个名称变更事件类型常量。UsernameChanged 事件不设——username 系登录锚点
// 不可改，无变更路径。
const (
	DeptNameChanged     Type = "DEPT_NAME_CHANGED"
	DictTypeNameChanged Type = "DICT_TYPE_NAME_CHANGED"
	MenuNameChanged     Type = "MENU_NAME_CHANGED"
	RoleNameChanged     Type = "ROLE_NAME_CHANGED"
	RealNameChanged     Type = "USER_REAL_NAME_CHANGED"
)

// NameChangedEvent 5 事件共用载荷（id+oldName+newName
// 三元组）。UPDATE 以 ID 为锚不依赖 OldName——载荷仅服务端构造（查库旧值 + 请求
// 新值），无客户端直传字段。
type NameChangedEvent struct {
	Type    Type
	ID      int64  // 实体主键
	OldName string // 发布侧空值兜底 ""
	NewName string
}

// Publisher service 层依赖的最小发布接口（依赖倒置 mock 点；ctx 供 handler 的
// db.WithContext 传递身份与取消信号）。
type Publisher interface {
	Publish(ctx context.Context, evt NameChangedEvent) error
}
