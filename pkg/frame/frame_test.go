package frame

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// fakeComponent 用于单测：记录 Start/Stop 调用次数，并支持模拟启动失败。
type fakeComponent struct {
	startCount atomic.Int32
	stopCount  atomic.Int32
	failStart  bool
}

func (c *fakeComponent) Start(ctx context.Context) error {
	if c.failStart {
		return errors.New("boom")
	}
	c.startCount.Add(1)
	return nil
}

func (c *fakeComponent) Stop(ctx context.Context) error {
	c.stopCount.Add(1)
	return nil
}

// TestRegisterSingletonAllowsFirstCall 验证首次 RegisterSingleton 会注册并参与 Start/Stop。
func TestRegisterSingletonAllowsFirstCall(t *testing.T) {
	f := New()
	c := &fakeComponent{}
	f.RegisterSingleton("cron", c)

	if err := f.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if c.startCount.Load() != 1 {
		t.Fatalf("startCount = %d, want 1", c.startCount.Load())
	}
}

// TestRegisterSingletonPanicsOnDuplicateKey 验证同一 key 重复 RegisterSingleton 立即 panic。
func TestRegisterSingletonPanicsOnDuplicateKey(t *testing.T) {
	f := New()
	f.RegisterSingleton("cron", &fakeComponent{})

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("重复的 key 应该让 RegisterSingleton panic")
		}
		msg, ok := r.(string)
		if !ok || msg == "" {
			t.Fatalf("panic 信息应该是非空字符串，实际: %v (%T)", r, r)
		}
	}()
	f.RegisterSingleton("cron", &fakeComponent{})
}

// TestRegisterSingletonDifferentKeysCoexist 验证不同 key 的 RegisterSingleton 互不影响。
func TestRegisterSingletonDifferentKeysCoexist(t *testing.T) {
	f := New()
	f.RegisterSingleton("cron", &fakeComponent{})
	f.RegisterSingleton("product-cache", &fakeComponent{}) // 不同 key，不应该 panic

	if err := f.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
}

// TestRegisterSingletonIndependentAcrossFrameInstances 验证不同 Frame 实例的同名 key 互不影响。
func TestRegisterSingletonIndependentAcrossFrameInstances(t *testing.T) {
	f1 := New()
	f2 := New()

	f1.RegisterSingleton("cron", &fakeComponent{})
	f2.RegisterSingleton("cron", &fakeComponent{}) // 不同 Frame 实例，同名 key 不应该 panic
}

func TestStartStopOrder(t *testing.T) {
	var order []string
	starter := func(name string) Component {
		return startStopFunc{
			start: func(context.Context) error { order = append(order, "start:"+name); return nil },
			stop:  func(context.Context) error { order = append(order, "stop:"+name); return nil },
		}
	}

	f := New()
	f.RegisterComponent(starter("a"))
	f.RegisterComponent(starter("b"))

	if err := f.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := f.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	want := []string{"start:a", "start:b", "stop:b", "stop:a"}
	if len(order) != len(want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order = %v, want %v", order, want)
		}
	}
}

func TestStartFailureRollsBackStartedComponents(t *testing.T) {
	ok := &fakeComponent{}
	bad := &fakeComponent{failStart: true}

	f := New()
	f.RegisterComponent(ok)
	f.RegisterComponent(bad)

	err := f.Start(context.Background())
	if err == nil {
		t.Fatal("expected Start() to fail")
	}
	if ok.startCount.Load() != 1 {
		t.Fatalf("ok.startCount = %d, want 1", ok.startCount.Load())
	}
	if ok.stopCount.Load() != 1 {
		t.Fatalf("ok component should be rolled back via Stop(), stopCount = %d", ok.stopCount.Load())
	}
}

func TestRestartStopsThenStartsAgain(t *testing.T) {
	c := &fakeComponent{}
	f := New()
	f.RegisterComponent(c)

	if err := f.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := f.Restart(context.Background()); err != nil {
		t.Fatalf("Restart() error = %v", err)
	}

	if c.startCount.Load() != 2 {
		t.Fatalf("startCount = %d, want 2 (initial Start + Restart)", c.startCount.Load())
	}
	if c.stopCount.Load() != 1 {
		t.Fatalf("stopCount = %d, want 1 (Restart's internal Stop)", c.stopCount.Load())
	}
}

func TestRunContextGracefulShutdownOnContextCancel(t *testing.T) {
	c := &fakeComponent{}
	f := New(WithShutdownTimeout(time.Second))
	f.RegisterComponent(c)

	var afterStartCalled, beforeStopCalled atomic.Bool
	f.AfterStart(func(context.Context) error { afterStartCalled.Store(true); return nil })
	f.BeforeStop(func(context.Context) error { beforeStopCalled.Store(true); return nil })

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- f.RunContext(ctx) }()

	// 给 RunContext 一点时间跑到"等待信号"阶段，再取消 ctx 模拟外部关闭请求。
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("RunContext() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("RunContext() did not return after context cancellation")
	}

	if !afterStartCalled.Load() {
		t.Error("AfterStart hook was not called")
	}
	if !beforeStopCalled.Load() {
		t.Error("BeforeStop hook was not called")
	}
	if c.stopCount.Load() != 1 {
		t.Errorf("component stopCount = %d, want 1", c.stopCount.Load())
	}
}

// startStopFunc 是一个用函数字面量实现 Component 接口的小工具，方便在单测里断言启停顺序。
type startStopFunc struct {
	start func(context.Context) error
	stop  func(context.Context) error
}

func (f startStopFunc) Start(ctx context.Context) error { return f.start(ctx) }
func (f startStopFunc) Stop(ctx context.Context) error  { return f.stop(ctx) }
