package refreshcache

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
)

type demoData struct {
	Value int `json:"value"`
}

// fakeRedis 是只实现 Get/Set 的内存 redis.Cmdable。
type fakeRedis struct {
	redis.Cmdable

	mu        sync.Mutex
	store     map[string]string
	available atomic.Bool
}

func newFakeRedis() *fakeRedis {
	f := &fakeRedis{store: make(map[string]string)}
	f.available.Store(true)
	return f
}

func (f *fakeRedis) Get(_ context.Context, key string) *redis.StringCmd {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.store[key]
	if !ok {
		return redis.NewStringResult("", redis.Nil)
	}
	return redis.NewStringResult(v, nil)
}

func (f *fakeRedis) Set(_ context.Context, key string, value any, _ time.Duration) *redis.StatusCmd {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.store[key] = value.(string)
	return redis.NewStatusResult("OK", nil)
}

func (f *fakeRedis) setRaw(key string, v any) {
	b, _ := json.Marshal(v)
	f.mu.Lock()
	f.store[key] = string(b)
	f.mu.Unlock()
}

func redisAccessor(client *fakeRedis) func() (redis.Cmdable, bool) {
	return func() (redis.Cmdable, bool) {
		if !client.available.Load() {
			return nil, false
		}
		return client, true
	}
}

// TestCacheStartPanicsWithoutRequiredOptions 验证缺少 Loader/Interval 时 Start 会 panic。
func TestCacheStartPanicsWithoutRequiredOptions(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("缺少必填项时 Start 应该 panic")
		}
	}()
	c := New(Options[demoData]{Key: "x"}) // 缺 Loader/RedisInterval/MemoryInterval
	_ = c.Start(context.Background())
}

// TestCollectorsAvailableBeforeStart 覆盖之前的 bug：内部 *cron.Component（连带它的
// Prometheus 指标对象）曾经要等 Start() 才创建，而调用方常见的用法是"New 之后立刻
// MustRegister(Collectors())"（见 cmd/example/bootstrap/refreshcache.go），这一步发生
// 在 Frame.Start 之前——Collectors() 在那个时间点只能拿到 nil，MustRegister 等于注册了
// 0 个指标，这个缓存的刷新任务指标会一直"注册成功"但实际不存在，且没有任何报错提示
// 这里出了问题。
func TestCollectorsAvailableBeforeStart(t *testing.T) {
	c := New(Options[demoData]{
		Key:            "collectors-before-start",
		Loader:         func(ctx context.Context) (demoData, error) { return demoData{}, nil },
		RedisInterval:  time.Second,
		MemoryInterval: time.Second,
	})

	if collectors := c.Collectors(); len(collectors) == 0 {
		t.Fatal("Collectors() 在 Start() 之前调用也应该返回真正的指标采集器，不应该是空的")
	}
}

// TestMultipleCacheInstancesCanRegisterCollectorsTogether 验证两个 Cache 实例的
// Collectors() 能一起注册进同一个 Registry，不会因为内部 cron.Component 撞名冲突。
func TestMultipleCacheInstancesCanRegisterCollectorsTogether(t *testing.T) {
	c1 := New(Options[demoData]{
		Key:            "multi-cache-test-1",
		Loader:         func(ctx context.Context) (demoData, error) { return demoData{}, nil },
		RedisInterval:  time.Second,
		MemoryInterval: time.Second,
	})
	c2 := New(Options[demoData]{
		Key:            "multi-cache-test-2",
		Loader:         func(ctx context.Context) (demoData, error) { return demoData{}, nil },
		RedisInterval:  time.Second,
		MemoryInterval: time.Second,
	})

	reg := prometheus.NewRegistry()
	for _, collector := range c1.Collectors() {
		if err := reg.Register(collector); err != nil {
			t.Fatalf("注册第一个 Cache 实例的指标失败: %v", err)
		}
	}
	for _, collector := range c2.Collectors() {
		if err := reg.Register(collector); err != nil {
			t.Fatalf("注册第二个 Cache 实例的指标失败（说明两个 Cache 内部的 cron.Component 撞名了）: %v", err)
		}
	}
}

// TestGetReturnsFalseBeforeAnyLoadSucceeds 验证从未加载成功时 Get 返回 (zero, false)。
func TestGetReturnsFalseBeforeAnyLoadSucceeds(t *testing.T) {
	c := New(Options[demoData]{
		Loader: func(ctx context.Context) (demoData, error) { return demoData{}, errors.New("source down") },
	})

	v, ok := c.Get(context.Background())
	if ok {
		t.Fatalf("应该返回 ok=false，实际拿到了 %+v", v)
	}
}

// TestCacheWarmUpUsesLoaderWhenRedisEmpty 验证 Redis 为空时冷启动走 Loader，Start 后立即可 Get。
func TestCacheWarmUpUsesLoaderWhenRedisEmpty(t *testing.T) {
	client := newFakeRedis()
	var loaderCalls atomic.Int32

	c := New(Options[demoData]{
		Key:            "warmup-key-1",
		Loader:         func(ctx context.Context) (demoData, error) { loaderCalls.Add(1); return demoData{Value: 1}, nil },
		RedisInterval:  time.Second,
		MemoryInterval: time.Second,
		Redis:          redisAccessor(client),
	})

	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start 失败: %v", err)
	}
	defer c.Stop(context.Background())

	v, ok := c.Get(context.Background())
	if !ok || v.Value != 1 {
		t.Fatalf("冷启动之后应该立刻拿到数据源的值，got ok=%v v=%+v", ok, v)
	}
	if loaderCalls.Load() == 0 {
		t.Fatal("Loader 应该被调用过（冷启动读数据源）")
	}
}

// TestCacheWarmUpPrefersRedisWhenAvailable 验证 Redis 已有数据时冷启动不再调用 Loader。
func TestCacheWarmUpPrefersRedisWhenAvailable(t *testing.T) {
	client := newFakeRedis()
	client.setRaw("warmup-key-2", demoData{Value: 99})

	var loaderCalls atomic.Int32
	c := New(Options[demoData]{
		Key:            "warmup-key-2",
		Loader:         func(ctx context.Context) (demoData, error) { loaderCalls.Add(1); return demoData{Value: 1}, nil },
		RedisInterval:  time.Second,
		MemoryInterval: time.Second,
		Redis:          redisAccessor(client),
	})

	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start 失败: %v", err)
	}
	defer c.Stop(context.Background())

	v, ok := c.Get(context.Background())
	if !ok || v.Value != 99 {
		t.Fatalf("应该优先使用 Redis 里已有的值 99，got ok=%v v=%+v", ok, v)
	}
	if loaderCalls.Load() != 0 {
		t.Fatal("Redis 已经有数据时，冷启动不应该再调用 Loader")
	}
}

// TestCacheRefreshesSourceToRedisPeriodically 验证会按 RedisInterval 把数据源写入 Redis。
func TestCacheRefreshesSourceToRedisPeriodically(t *testing.T) {
	client := newFakeRedis()
	var current atomic.Int32
	current.Store(1)

	c := New(Options[demoData]{
		Key:            "refresh-key-1",
		Loader:         func(ctx context.Context) (demoData, error) { return demoData{Value: int(current.Load())}, nil },
		RedisInterval:  time.Second,
		MemoryInterval: time.Second,
		Redis:          redisAccessor(client),
	})

	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start 失败: %v", err)
	}
	defer c.Stop(context.Background())

	current.Store(42)                   // 数据源的值变了
	time.Sleep(1300 * time.Millisecond) // 等一轮 RedisInterval 触发

	raw, err := client.Get(context.Background(), "refresh-key-1").Result()
	if err != nil {
		t.Fatalf("Redis 里应该有值: %v", err)
	}
	var v demoData
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatalf("Unmarshal 失败: %v", err)
	}
	if v.Value != 42 {
		t.Fatalf("Redis 里的值应该已经刷新成 42，实际 %d", v.Value)
	}
}

// TestCacheRefreshesMemoryFromRedis 验证会按 MemoryInterval 把 Redis 最新值同步进内存。
func TestCacheRefreshesMemoryFromRedis(t *testing.T) {
	client := newFakeRedis()
	client.setRaw("refresh-key-2", demoData{Value: 1})

	c := New(Options[demoData]{
		Key:    "refresh-key-2",
		Loader: func(ctx context.Context) (demoData, error) { return demoData{Value: 1}, nil },
		// RedisInterval 故意长于测试窗口，避免 DB→Redis 覆盖手动写入的值。
		RedisInterval:  10 * time.Second,
		MemoryInterval: time.Second,
		Redis:          redisAccessor(client),
	})

	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start 失败: %v", err)
	}
	defer c.Stop(context.Background())

	client.setRaw("refresh-key-2", demoData{Value: 77}) // 模拟别的进程更新了 Redis
	time.Sleep(1300 * time.Millisecond)                 // 等一轮 MemoryInterval 触发

	v, ok := c.Get(context.Background())
	if !ok || v.Value != 77 {
		t.Fatalf("内存应该已经同步 Redis 的最新值 77，got ok=%v v=%+v", ok, v)
	}
}

// TestCacheFallsBackToLoaderWhenRedisUnavailable 验证 Redis 不可用时回退调用 Loader。
func TestCacheFallsBackToLoaderWhenRedisUnavailable(t *testing.T) {
	client := newFakeRedis()
	client.setRaw("fallback-key-1", demoData{Value: 1})

	var sourceValue atomic.Int32
	sourceValue.Store(1)

	c := New(Options[demoData]{
		Key:            "fallback-key-1",
		Loader:         func(ctx context.Context) (demoData, error) { return demoData{Value: int(sourceValue.Load())}, nil },
		RedisInterval:  time.Second,
		MemoryInterval: time.Second,
		Redis:          redisAccessor(client),
	})

	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start 失败: %v", err)
	}
	defer c.Stop(context.Background())

	client.available.Store(false) // Redis 突然不可用
	sourceValue.Store(123)        // 这段时间数据源的值变了
	time.Sleep(1300 * time.Millisecond)

	v, ok := c.Get(context.Background())
	if !ok || v.Value != 123 {
		t.Fatalf("Redis 不可用时应该回退到数据源，拿到最新值 123，got ok=%v v=%+v", ok, v)
	}
}
