package alert

import (
	"context"
	"sync"
	"testing"
	"time"
)

// TestNotifyIsNoopWithoutHook 验证未注册 Hook 时 Notify 是 no-op，不 panic。
func TestNotifyIsNoopWithoutHook(t *testing.T) {
	SetHook(nil)
	Notify(context.Background(), Event{Scope: "test"})
}

// TestNotifyCallsRegisteredHook 验证注册 Hook 后会被调用并收到完整 Event。
func TestNotifyCallsRegisteredHook(t *testing.T) {
	defer SetHook(nil)

	var mu sync.Mutex
	var got Event
	done := make(chan struct{})

	SetHook(func(ctx context.Context, e Event) {
		mu.Lock()
		got = e
		mu.Unlock()
		close(done)
	})

	Notify(context.Background(), Event{Scope: "cron", Name: "heartbeat", Message: "task failed"})

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Hook 应该被调用到")
	}

	mu.Lock()
	defer mu.Unlock()
	if got.Scope != "cron" || got.Name != "heartbeat" || got.Message != "task failed" {
		t.Fatalf("Hook 收到的 Event 不对: %+v", got)
	}
}

// TestNotifyRecoversFromHookPanic 验证 Hook panic 不会传播出 Notify。
func TestNotifyRecoversFromHookPanic(t *testing.T) {
	defer SetHook(nil)

	done := make(chan struct{})
	SetHook(func(ctx context.Context, e Event) {
		defer close(done)
		panic("boom")
	})

	Notify(context.Background(), Event{Scope: "test"})

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Hook 应该被调用到（即使它自己会 panic）")
	}
}

// TestNotifyDetachesFromCanceledContext 验证传入已取消 ctx 时 Hook 仍能拿到未取消的 ctx。
func TestNotifyDetachesFromCanceledContext(t *testing.T) {
	defer SetHook(nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 调用 Notify 之前就取消

	done := make(chan struct{})
	var sawDone bool
	SetHook(func(ctx context.Context, e Event) {
		defer close(done)
		select {
		case <-ctx.Done():
			sawDone = true
		default:
		}
	})

	Notify(ctx, Event{Scope: "test"})

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Hook 应该被调用到")
	}
	if sawDone {
		t.Fatal("传给 Hook 的 ctx 应该已经剥离取消信号，不应该表现为 Done")
	}
}

// TestSetHookNilClearsHook 验证 SetHook(nil) 会清空已注册的 Hook。
func TestSetHookNilClearsHook(t *testing.T) {
	called := false
	SetHook(func(ctx context.Context, e Event) { called = true })
	SetHook(nil)

	Notify(context.Background(), Event{Scope: "test"})
	time.Sleep(50 * time.Millisecond)

	if called {
		t.Fatal("SetHook(nil) 之后不应该再调用旧的 Hook")
	}
}
