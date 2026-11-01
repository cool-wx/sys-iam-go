package event

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// TestPublishInvokesAllListenersSynchronously 注册 2 个 listener 后 Publish：
// 两者同步执行（调用返回前完成）且各自收到完整载荷。
func TestPublishInvokesAllListenersSynchronously(t *testing.T) {
	d := NewDispatcher()
	var mu sync.Mutex
	var got []NameChangedEvent
	done := make(chan struct{}, 2)
	l1 := func(_ context.Context, evt NameChangedEvent) error {
		mu.Lock()
		got = append(got, evt)
		mu.Unlock()
		done <- struct{}{}
		return nil
	}
	l2 := func(_ context.Context, evt NameChangedEvent) error {
		mu.Lock()
		got = append(got, evt)
		mu.Unlock()
		done <- struct{}{}
		return nil
	}
	d.Register(DeptNameChanged, l1)
	d.Register(DeptNameChanged, l2)

	evt := NameChangedEvent{Type: DeptNameChanged, ID: 42, OldName: "旧名", NewName: "新名"}
	if err := d.Publish(context.Background(), evt); err != nil {
		t.Fatalf("Publish 返回错误: %v", err)
	}
	// 同步语义直证：Publish 返回时 listener 必已执行完（无需等待/轮询）——
	// 若实现为异步派发，此处的 channel 计数在无超时读取下会死锁而非失败。
	for i := 0; i < 2; i++ {
		<-done
	}
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 2 {
		t.Fatalf("期望 2 个 listener 均执行，实际 %d", len(got))
	}
	for _, g := range got {
		if g != evt {
			t.Fatalf("载荷不完整: got=%+v want=%+v", g, evt)
		}
	}
}

// TestPublishReturnsFirstErrorAndStops 首个 listener 返回 error：Publish 返回该
// 错误且第二个 listener 不执行（首错即返——错误沿调用链上抛为 API 500 的前提）。
func TestPublishReturnsFirstErrorAndStops(t *testing.T) {
	d := NewDispatcher()
	sentinel := errors.New("同步 UPDATE 失败")
	secondCalled := false
	d.Register(RoleNameChanged, func(_ context.Context, _ NameChangedEvent) error {
		return sentinel
	})
	d.Register(RoleNameChanged, func(_ context.Context, _ NameChangedEvent) error {
		secondCalled = true
		return nil
	})
	err := d.Publish(context.Background(), NameChangedEvent{Type: RoleNameChanged, ID: 1, NewName: "x"})
	if !errors.Is(err, sentinel) {
		t.Fatalf("期望返回首个 listener 的错误 sentinel，实际: %v", err)
	}
	if secondCalled {
		t.Fatal("首错后第二个 listener 不应被执行")
	}
}

// TestPublishUnregisteredTypeReturnsNil 未注册的事件类型：Publish 返回 nil
// （无订阅者即无事发生，不视为错误）。
func TestPublishUnregisteredTypeReturnsNil(t *testing.T) {
	d := NewDispatcher()
	if err := d.Publish(context.Background(), NameChangedEvent{Type: MenuNameChanged, ID: 1, NewName: "x"}); err != nil {
		t.Fatalf("未注册类型 Publish 应返回 nil，实际: %v", err)
	}
}

// TestRegisterConcurrentSafe 并发 Register + Publish 无数据竞争（go test -race
// 覆盖面；RWMutex 写锁注册、快照后调用）。
func TestRegisterConcurrentSafe(t *testing.T) {
	d := NewDispatcher()
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d.Register(RealNameChanged, func(_ context.Context, _ NameChangedEvent) error { return nil })
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = d.Publish(context.Background(), NameChangedEvent{Type: RealNameChanged, ID: 1, NewName: "x"})
		}()
	}
	wg.Wait()
}
