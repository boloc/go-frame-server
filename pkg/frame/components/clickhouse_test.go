package components

import (
	"context"
	"testing"
	"time"
)

func TestNewClickHouseComponentConnectRetryDefaults(t *testing.T) {
	c := NewClickHouseComponent("ch_test_defaults_"+t.Name(), false)
	if c.config.ConnectRetryAttempts != 3 {
		t.Errorf("ConnectRetryAttempts default = %d, want 3", c.config.ConnectRetryAttempts)
	}
	if c.config.ConnectRetryInterval != 2*time.Second {
		t.Errorf("ConnectRetryInterval default = %v, want 2s", c.config.ConnectRetryInterval)
	}
}

func TestWithClickHouseConnectRetryOptionsOverrideDefaults(t *testing.T) {
	c := NewClickHouseComponent("ch_test_override_"+t.Name(), false,
		WithClickHouseConnectRetryAttempts(5),
		WithClickHouseConnectRetryInterval(500*time.Millisecond),
	)
	if c.config.ConnectRetryAttempts != 5 {
		t.Errorf("ConnectRetryAttempts = %d, want 5", c.config.ConnectRetryAttempts)
	}
	if c.config.ConnectRetryInterval != 500*time.Millisecond {
		t.Errorf("ConnectRetryInterval = %v, want 500ms", c.config.ConnectRetryInterval)
	}
}

// TestNewClickHouseComponentPanicsOnDuplicateName 验证重复注册同名实例会 panic。
func TestNewClickHouseComponentPanicsOnDuplicateName(t *testing.T) {
	name := "ch_test_duplicate_" + t.Name()
	NewClickHouseComponent(name, false)

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic when registering the same ClickHouse instance name twice")
		}
	}()
	NewClickHouseComponent(name, false)
}

// TestClickHouseComponentRetriesOnStartFailure 验证连不上时 Start 会按 attempts/interval 重试。
func TestClickHouseComponentRetriesOnStartFailure(t *testing.T) {
	c := NewClickHouseComponent("ch_test_retry_"+t.Name(), false,
		WithClickHouseAddress([]string{"127.0.0.1:1"}), // 必然连不上，且拒绝得很快
		WithClickHouseDialTimeout(200*time.Millisecond),
		WithClickHouseConnectRetryAttempts(3),
		WithClickHouseConnectRetryInterval(20*time.Millisecond),
	)

	start := time.Now()
	err := c.Start(context.Background())
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected Start() to fail against an unreachable address")
	}
	if elapsed < 30*time.Millisecond {
		t.Fatalf("elapsed = %v，3 次重试、间隔 20ms 应该至少耗时 40ms 左右，看起来没有真正重试", elapsed)
	}
}

// TestClickHouseComponentStopWithoutStartIsNoop 验证未 Start 时 Stop 是安全 no-op。
func TestClickHouseComponentStopWithoutStartIsNoop(t *testing.T) {
	c := NewClickHouseComponent("ch_test_stop_noop_"+t.Name(), false)
	if err := c.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() 在没 Start 过时应该是 no-op，got error: %v", err)
	}
}
