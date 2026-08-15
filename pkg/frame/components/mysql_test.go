package components

import (
	"context"
	"os"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestMaskDSN(t *testing.T) {
	cases := map[string]string{
		"user:secret@tcp(127.0.0.1:3306)/db": "user:***@tcp(127.0.0.1:3306)/db",
		"nocolonoratsign":                    "nocolonoratsign",
		"":                                   "",
	}
	for in, want := range cases {
		if got := maskDSN(in); got != want {
			t.Errorf("maskDSN(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNewMySQLComponentPanicsOnDuplicateName(t *testing.T) {
	const name = "mysql_test_duplicate"
	NewMySQLComponent(name, &MySQLConfig{MasterDSN: "unused"}, false)

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic when registering the same MySQL instance name twice")
		}
	}()
	NewMySQLComponent(name, &MySQLConfig{MasterDSN: "unused-again"}, false)
}

// TestTryGetMySQLComponentDoesNotPanicWhenMissing 验证未注册实例时 TryGet 返回 (nil, false)，不 panic。
func TestTryGetMySQLComponentDoesNotPanicWhenMissing(t *testing.T) {
	if _, ok := TryGetMySQLComponent("does-not-exist-" + t.Name()); ok {
		t.Fatal("expected ok=false for an instance name that was never registered")
	}
}

func TestGetMySQLComponentPanicsWhenMissing(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected GetMySQLComponent to panic for an unregistered instance name")
		}
	}()
	GetMySQLComponent("does-not-exist-" + t.Name())
}

func TestTryDefaultMasterDBReturnsFalseWhenNotRegistered(t *testing.T) {
	// 只断言无默认实例时不 panic，不改动包级 defaultMySQLDB。
	if defaultMySQLDB == nil {
		if _, ok := TryDefaultMasterDB(); ok {
			t.Fatal("expected ok=false when there is no default MySQL instance")
		}
	}
}

func TestSlaveFallsBackToMasterWhenNoReplicas(t *testing.T) {
	m := &MySQLComponent{config: &MySQLConfig{}}
	// 用占位 *gorm.DB 验证无从库时 Slave() 回落到主库。
	fakeMaster := &gorm.DB{}
	m.master = fakeMaster
	if got := m.Slave(); got != fakeMaster {
		t.Fatal("Slave() 应该在没有从库时回落到主库")
	}
}

// TestSlaveDoesNotPanicWhenReplicasClearedConcurrently 用 -race 跑高并发的
// Slave()/清空 replicas 组合，确认不会 data race、也不会 panic。
func TestSlaveDoesNotPanicWhenReplicasClearedConcurrently(t *testing.T) {
	m := &MySQLComponent{config: &MySQLConfig{}}
	m.master = &gorm.DB{}
	m.replicas = []*gorm.DB{{}, {}, {}}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 1000; i++ {
			if got := m.Slave(); got == nil {
				t.Error("Slave() 不应该返回 nil：无从库时应该回落到主库")
			}
		}
	}()

	for i := 0; i < 1000; i++ {
		m.mu.Lock()
		if i%2 == 0 {
			m.replicas = nil
		} else {
			m.replicas = []*gorm.DB{{}, {}}
		}
		m.mu.Unlock()
	}
	<-done
}

// TestTryMasterDBReportsUnavailableWhenRegisteredButNotConnected 验证实例已注册但
// 未连接时，TryMasterDB 返回 (nil, false)，MasterDB 会 panic，而不是返回一个 nil 的
// *gorm.DB 让调用方在别处踩坑。
func TestTryMasterDBReportsUnavailableWhenRegisteredButNotConnected(t *testing.T) {
	const name = "mysql_test_registered_not_connected"
	NewMySQLComponent(name, &MySQLConfig{MasterDSN: "unused"}, false)

	// 不调用 Start；模拟"已注册、但当前没有可用连接"（Stop 之后、或者 Restart 重连
	// 完成之前）的状态。
	if db, ok := TryMasterDB(name); ok || db != nil {
		t.Fatalf("TryMasterDB() = (%v, %v)，实例已注册但未连接时应该返回 (nil, false)", db, ok)
	}

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected MasterDB() to panic when the instance is registered but not currently connected")
		}
	}()
	MasterDB(name)
}

// TestMySQLIntegration 在设置 GFS_TEST_MYSQL_DSN 时验证真实连接、从库容错和连接池参数。
func TestMySQLIntegration(t *testing.T) {
	dsn := os.Getenv("GFS_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("GFS_TEST_MYSQL_DSN 未设置，跳过需要真实 MySQL 的集成测试")
	}

	t.Run("healthy master + one bad slave + one healthy slave", func(t *testing.T) {
		m := &MySQLComponent{
			config: &MySQLConfig{
				MasterDSN: dsn,
				// 127.0.0.1:1 上不会有任何服务监听，制造一个必然失败、但拒绝得很快的"坏从库"。
				SlavesDSN:            []string{"root@tcp(127.0.0.1:1)/testdb", dsn},
				MaxOpenConns:         5,
				MaxIdleConns:         2,
				ConnMaxIdleTime:      time.Minute,
				ConnectRetryAttempts: 1, // 坏地址不需要重试也能验证"跳过不致命"，重试逻辑单独测
			},
		}
		if err := m.Start(context.Background()); err != nil {
			t.Fatalf("Start() 不应该因为一个从库连不上而失败: %v", err)
		}
		defer m.Stop(context.Background())

		if m.Master() == nil {
			t.Fatal("Master() 应该是一个可用连接")
		}
		if len(m.replicas) != 1 {
			t.Fatalf("replicas 数量 = %d, want 1（一个坏从库应该被跳过，一个好从库应该保留）", len(m.replicas))
		}
	})

	t.Run("retry actually retries before giving up", func(t *testing.T) {
		m := &MySQLComponent{
			config: &MySQLConfig{
				MasterDSN:            "root@tcp(127.0.0.1:1)/testdb", // 必然连不上
				ConnectRetryAttempts: 3,
				ConnectRetryInterval: 200 * time.Millisecond,
			},
		}

		start := time.Now()
		err := m.Start(context.Background())
		elapsed := time.Since(start)

		if err == nil {
			t.Fatal("expected Start() to fail against an unreachable address")
		}
		// 3 次尝试之间有 2 次等待间隔，总耗时应该 >= 2*200ms（留一点余量，不要求精确）。
		if elapsed < 300*time.Millisecond {
			t.Fatalf("elapsed = %v, 看起来没有真的重试（预期至少 ~400ms）", elapsed)
		}
	})

	t.Run("connection pool settings are applied", func(t *testing.T) {
		m := &MySQLComponent{
			config: &MySQLConfig{
				MasterDSN:            dsn,
				MaxOpenConns:         7,
				MaxIdleConns:         3,
				ConnectRetryAttempts: 1,
			},
		}
		if err := m.Start(context.Background()); err != nil {
			t.Fatalf("Start() error = %v", err)
		}
		defer m.Stop(context.Background())

		sqlDB, err := m.Master().DB()
		if err != nil {
			t.Fatalf("DB() error = %v", err)
		}
		stats := sqlDB.Stats()
		if stats.MaxOpenConnections != 7 {
			t.Fatalf("MaxOpenConnections = %d, want 7", stats.MaxOpenConnections)
		}
	})
}
