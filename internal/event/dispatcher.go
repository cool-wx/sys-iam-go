package event

import (
	"context"
	"sync"
)

// Listener 事件监听器签名：同步执行于发布者调用栈（非异步派发，D-02 明文——
// 异步会使 handler 失败脱离请求错误链，"500 + 重试自愈"语义崩塌）。
type Listener func(ctx context.Context, evt NameChangedEvent) error

// Dispatcher 按事件类型注册的同步分发器：map[Type][]Listener 直查 + 逐个同步
// 调用。约 60 行自建（D-01），禁事件总线框架/异步队列；Register 并发安全
// （RWMutex），Publish 快照后调用——监听器执行不持锁。
type Dispatcher struct {
	mu        sync.RWMutex
	listeners map[Type][]Listener
}

// Dispatcher 编译期实现 Publisher 契约（service 层依赖接口而非具体类型）。
var _ Publisher = (*Dispatcher)(nil)

// NewDispatcher 构造空注册表。
func NewDispatcher() *Dispatcher {
	return &Dispatcher{listeners: make(map[Type][]Listener)}
}

// Register 向指定事件类型追加监听器（app.New 装配期调用，RegisterAll 收口）。
func (d *Dispatcher) Register(t Type, l Listener) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.listeners == nil {
		d.listeners = make(map[Type][]Listener)
	}
	d.listeners[t] = append(d.listeners[t], l)
}

// Publish 同步逐个调用该事件类型的全部监听器：首个错误即返回（→ service →
// API 500，D-03）；未注册的事件类型直接返回 nil（无订阅者即无事发生）。调用前
// 拍监听器快照，执行阶段不持锁——handler 内的 DB 事务不阻塞并发注册。
func (d *Dispatcher) Publish(ctx context.Context, evt NameChangedEvent) error {
	d.mu.RLock()
	ls := make([]Listener, len(d.listeners[evt.Type]))
	copy(ls, d.listeners[evt.Type])
	d.mu.RUnlock()
	for _, l := range ls {
		if err := l(ctx, evt); err != nil {
			return err
		}
	}
	return nil
}
