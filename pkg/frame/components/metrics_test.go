package components

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
)

func collectAll(t *testing.T, c prometheus.Collector) []prometheus.Metric {
	t.Helper()
	ch := make(chan prometheus.Metric, 64)
	done := make(chan struct{})
	var metrics []prometheus.Metric
	go func() {
		defer close(done)
		for m := range ch {
			metrics = append(metrics, m)
		}
	}()
	c.Collect(ch)
	close(ch)
	<-done
	return metrics
}

func TestMySQLPoolCollectorDescribeSendsAllDescs(t *testing.T) {
	m := &MySQLComponent{config: &MySQLConfig{}}
	c := NewMySQLPoolCollector(map[string]*MySQLComponent{"test_db": m})

	ch := make(chan *prometheus.Desc, 64)
	go func() {
		c.Describe(ch)
		close(ch)
	}()

	var count int
	for range ch {
		count++
	}
	const wantDescs = 9 // maxOpen/open/inUse/idle/waitCount/waitDuration/maxIdleClosed/maxIdleTimeClosed/maxLifetimeClosed
	if count != wantDescs {
		t.Fatalf("Describe() sent %d descs, want %d", count, wantDescs)
	}
}

// TestMySQLPoolCollectorCollectWithoutMasterDoesNotPanic 验证 master 为 nil 时 Collect 不 panic、不产出指标。
func TestMySQLPoolCollectorCollectWithoutMasterDoesNotPanic(t *testing.T) {
	m := &MySQLComponent{config: &MySQLConfig{}}
	c := NewMySQLPoolCollector(map[string]*MySQLComponent{"test_db": m})

	metrics := collectAll(t, c)
	if len(metrics) != 0 {
		t.Fatalf("Collect() produced %d metrics for a component with no master, want 0", len(metrics))
	}
}

// TestMySQLPoolCollectorRegisterMultipleInstancesOnce 三个命名实例必须只注册一份 Collector。
func TestMySQLPoolCollectorRegisterMultipleInstancesOnce(t *testing.T) {
	reg := prometheus.NewRegistry()
	c := NewMySQLPoolCollector(map[string]*MySQLComponent{
		"default_db": {config: &MySQLConfig{}},
		"config_db":  {config: &MySQLConfig{}},
		"log_db":     {config: &MySQLConfig{}},
	})
	if err := reg.Register(c); err != nil {
		t.Fatalf("Register() one collector for three instances: %v", err)
	}
}

type fakeRedisPoolStatsProvider struct {
	stats *redis.PoolStats
}

func (f *fakeRedisPoolStatsProvider) PoolStats() *redis.PoolStats { return f.stats }

func TestRedisPoolCollectorDescribeSendsAllDescs(t *testing.T) {
	c := NewRedisPoolCollector("default", &fakeRedisPoolStatsProvider{})

	ch := make(chan *prometheus.Desc, 64)
	go func() {
		c.Describe(ch)
		close(ch)
	}()

	var count int
	for range ch {
		count++
	}
	const wantDescs = 6 // hits/misses/timeouts/totalConns/idleConns/staleConns
	if count != wantDescs {
		t.Fatalf("Describe() sent %d descs, want %d", count, wantDescs)
	}
}

func TestRedisPoolCollectorCollectWithNilStatsDoesNotPanic(t *testing.T) {
	c := NewRedisPoolCollector("default", &fakeRedisPoolStatsProvider{stats: nil})

	metrics := collectAll(t, c)
	if len(metrics) != 0 {
		t.Fatalf("Collect() produced %d metrics for nil PoolStats, want 0", len(metrics))
	}
}

func TestRedisPoolCollectorCollectWithStats(t *testing.T) {
	c := NewRedisPoolCollector("default", &fakeRedisPoolStatsProvider{stats: &redis.PoolStats{
		Hits:       10,
		Misses:     2,
		Timeouts:   1,
		TotalConns: 5,
		IdleConns:  3,
		StaleConns: 0,
	}})

	metrics := collectAll(t, c)
	if len(metrics) != 6 {
		t.Fatalf("Collect() produced %d metrics, want 6", len(metrics))
	}
}

// TestRedisComponentsImplementPoolStatsProvider 验证三种 Redis 组件都实现 PoolStats 接口。
func TestRedisComponentsImplementPoolStatsProvider(t *testing.T) {
	var _ RedisPoolStatsProvider = &RedisComponent{}
	var _ RedisPoolStatsProvider = &RedisClusterComponent{}
	var _ RedisPoolStatsProvider = &RedisSentinelComponent{}
}
