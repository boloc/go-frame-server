package components

import (
	"context"
	"testing"
	"time"
)

func TestClickHouseGORMConfigApplyDefaults(t *testing.T) {
	cfg := &ClickHouseGORMConfig{}
	cfg.applyDefaults()

	if cfg.MaxIdleConns != 5 {
		t.Errorf("MaxIdleConns = %d, want 5", cfg.MaxIdleConns)
	}
	if cfg.MaxOpenConns != 10 {
		t.Errorf("MaxOpenConns = %d, want 10", cfg.MaxOpenConns)
	}
	if cfg.ConnectRetryAttempts != 3 {
		t.Errorf("ConnectRetryAttempts = %d, want 3", cfg.ConnectRetryAttempts)
	}
	if cfg.ConnectRetryInterval != 2*time.Second {
		t.Errorf("ConnectRetryInterval = %v, want 2s", cfg.ConnectRetryInterval)
	}
}

func TestNewClickHouseGORMComponentPanicsOnDuplicateName(t *testing.T) {
	name := "ch_gorm_test_duplicate_" + t.Name()
	NewClickHouseGORMComponent(name, &ClickHouseGORMConfig{}, false)

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic when registering the same ClickHouse GORM instance name twice")
		}
	}()
	NewClickHouseGORMComponent(name, &ClickHouseGORMConfig{}, false)
}

// TestClickHouseGORMComponentRetriesOnStartFailure 验证连不上时 Start 会按 attempts/interval 重试。
func TestClickHouseGORMComponentRetriesOnStartFailure(t *testing.T) {
	name := "ch_gorm_test_retry_" + t.Name()
	c := NewClickHouseGORMComponent(name, &ClickHouseGORMConfig{
		Address:              []string{"127.0.0.1:1"}, // 必然连不上，且拒绝得很快
		Database:             "testdb",
		DialTimeout:          200 * time.Millisecond,
		ConnectRetryAttempts: 3,
		ConnectRetryInterval: 20 * time.Millisecond,
	}, false)

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

func TestTryGetClickHouseGORMComponentDoesNotPanicWhenMissing(t *testing.T) {
	if _, ok := TryGetClickHouseGORMComponent("does-not-exist-" + t.Name()); ok {
		t.Fatal("expected ok=false for an instance name that was never registered")
	}
}

func TestGetClickHouseGORMComponentPanicsWhenMissing(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected GetClickHouseGORMComponent to panic for an unregistered instance name")
		}
	}()
	GetClickHouseGORMComponent("does-not-exist-" + t.Name())
}

func TestTryDefaultClickHouseDBReturnsFalseWhenNotRegistered(t *testing.T) {
	if defaultClickHouseGORM == nil {
		if _, ok := TryDefaultClickHouseDB(); ok {
			t.Fatal("expected ok=false when there is no default ClickHouse GORM instance")
		}
	}
}
