package components

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestNewRedisComponentConnectRetryDefaults(t *testing.T) {
	r := NewRedisComponent()
	if r.connectRetryAttempts != 3 {
		t.Errorf("connectRetryAttempts default = %d, want 3", r.connectRetryAttempts)
	}
	if r.connectRetryInterval != 2*time.Second {
		t.Errorf("connectRetryInterval default = %v, want 2s", r.connectRetryInterval)
	}
}

func TestWithRedisConnectRetryOptionsOverrideDefaults(t *testing.T) {
	r := NewRedisComponent(
		WithRedisConnectRetryAttempts(5),
		WithRedisConnectRetryInterval(500*time.Millisecond),
	)
	if r.connectRetryAttempts != 5 {
		t.Errorf("connectRetryAttempts = %d, want 5", r.connectRetryAttempts)
	}
	if r.connectRetryInterval != 500*time.Millisecond {
		t.Errorf("connectRetryInterval = %v, want 500ms", r.connectRetryInterval)
	}
}

// TestRedisComponentRetriesOnStartFailure 验证连不上时 Start 会按 attempts/interval 重试。
func TestRedisComponentRetriesOnStartFailure(t *testing.T) {
	r := NewRedisComponent(
		WithRedisAddr("127.0.0.1:1"), // 必然连不上，且拒绝得很快
		WithRedisConnectRetryAttempts(3),
		WithRedisConnectRetryInterval(20*time.Millisecond),
	)

	start := time.Now()
	err := r.Start(context.Background())
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected Start() to fail against an unreachable address")
	}
	if elapsed < 30*time.Millisecond {
		t.Fatalf("elapsed = %v，3 次重试、间隔 20ms 应该至少耗时 40ms 左右，看起来没有真正重试", elapsed)
	}
}

// TestRedisComponentDoesNotPublishGlobalUntilStartSucceeds 验证 Start 失败或未调用时不发布全局实例。
func TestRedisComponentDoesNotPublishGlobalUntilStartSucceeds(t *testing.T) {
	GlobalRedisComponent = nil // 隔离其它测试可能留下的状态

	// 只试一次，避免为验证“失败不发布全局实例”等待默认重试。
	r := NewRedisComponent(WithRedisAddr("127.0.0.1:1"), WithRedisConnectRetryAttempts(1)) // 必然连不上
	if GlobalRedisComponent != nil {
		t.Fatal("NewRedisComponent 不应该在构造阶段就发布全局实例")
	}

	if err := r.Start(context.Background()); err == nil {
		t.Fatal("expected Start() to fail against an unreachable address")
	}
	if GlobalRedisComponent != nil {
		t.Fatal("Start() 失败后不应该发布全局实例")
	}
}

// TestRedisIntegration 在设置 GFS_TEST_REDIS_ADDR 时验证 Start 发布全局实例、Stop 后清空。
func TestRedisIntegration(t *testing.T) {
	addr := os.Getenv("GFS_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("GFS_TEST_REDIS_ADDR 未设置，跳过需要真实 Redis 的集成测试")
	}

	GlobalRedisComponent = nil
	r := NewRedisComponent(WithRedisAddr(addr))

	if err := r.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if GlobalRedisComponent != r {
		t.Fatal("Start() 成功后应该把自己发布为全局实例")
	}
	if r.GetClient() == nil {
		t.Fatal("GetClient() 应该返回一个可用客户端")
	}

	if err := r.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if GlobalRedisComponent != nil {
		t.Fatal("Stop() 之后应该清空全局实例，不能让别人拿到一个已经 Close 的 client")
	}
	if r.GetClient() != nil {
		t.Fatal("Stop() 之后 GetClient() 应该返回 nil")
	}
}
