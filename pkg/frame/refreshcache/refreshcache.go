// Package refreshcache 提供双层定时刷新只读缓存：数据源 → Redis → 进程内存。
// Redis 不可用时内存层回退到数据源。适合读多写少、允许有界延迟的数据。
package refreshcache

import (
	"context"
	"encoding/json"
	"math/rand"
	"sync/atomic"
	"time"

	"github.com/boloc/go-frame-server/pkg/alert"
	"github.com/boloc/go-frame-server/pkg/frame"
	"github.com/boloc/go-frame-server/pkg/frame/cron"
	"github.com/boloc/go-frame-server/pkg/logger"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Loader 从数据源加载最新数据。
type Loader[T any] func(ctx context.Context) (T, error)

// Options 缓存配置。Key/Loader/RedisInterval/MemoryInterval 必须显式设置。
type Options[T any] struct {
	Key            string
	Loader         Loader[T]
	RedisInterval  time.Duration
	MemoryInterval time.Duration
	// RedisTTL 必须明显大于 RedisInterval，默认 RedisInterval * 5。
	RedisTTL time.Duration
	// Jitter 刷新前的随机抖动上限，多实例部署时用来错开刷新时机。
	Jitter time.Duration
	Redis  func() (redis.Cmdable, bool)
}

// Cache 双层刷新只读缓存，实现 frame.Component（Start(ctx)/Stop(ctx)）。
type Cache[T any] struct {
	opts Options[T]
	cron *cron.Component

	value atomic.Pointer[T]
}

// New 创建双层刷新只读缓存。必填项未设置时 Start 会 panic。
//
// 内部的 *cron.Component 在构造时就创建好，不等到 Start，这样 Collectors() 在 Start
// 之前调用也能拿到真正的指标对象（用 opts.Key 当 cron.Component 的 name，见
// cron.NewComponent 的文档注释）。
func New[T any](opts Options[T]) *Cache[T] {
	if opts.Redis == nil {
		opts.Redis = frame.TryGetRedisCmdable
	}
	c := &Cache[T]{opts: opts}
	c.cron = cron.NewComponent(opts.Key,
		cron.Task{
			Name:     "cache-refresh-source-to-redis:" + opts.Key,
			Schedule: everySchedule(opts.RedisInterval),
			Run:      c.refreshSourceToRedis,
		},
		cron.Task{
			Name:     "cache-refresh-redis-to-memory:" + opts.Key,
			Schedule: everySchedule(opts.MemoryInterval),
			Run:      c.refreshRedisToMemory,
		},
	)
	return c
}

// Get 优先读内存；内存为空时同步调用 Loader，不写回缓存。ok 为 false 表示两边都拿不到。
func (c *Cache[T]) Get(ctx context.Context) (T, bool) {
	if v := c.value.Load(); v != nil {
		return *v, true
	}

	v, err := c.opts.Loader(ctx)
	if err != nil {
		var zero T
		return zero, false
	}
	return v, true
}

// Start 校验配置、同步预热，然后启动两条刷新任务。
func (c *Cache[T]) Start(ctx context.Context) error {
	if c.opts.Key == "" || c.opts.Loader == nil || c.opts.RedisInterval <= 0 || c.opts.MemoryInterval <= 0 {
		panic("cache: Key/Loader/RedisInterval/MemoryInterval 都必须显式设置为非零值")
	}
	if c.opts.RedisTTL <= 0 {
		c.opts.RedisTTL = c.opts.RedisInterval * 5
	}

	c.warmUp(ctx)

	return c.cron.Start(ctx)
}

func (c *Cache[T]) Stop(ctx context.Context) error {
	return c.cron.Stop(ctx)
}

// Collectors 返回需要注册到 Prometheus 的采集器。
func (c *Cache[T]) Collectors() []prometheus.Collector {
	return c.cron.Collectors()
}

func (c *Cache[T]) warmUp(ctx context.Context) {
	if client, ok := c.opts.Redis(); ok {
		if raw, err := client.Get(ctx, c.opts.Key).Result(); err == nil {
			var v T
			if err := json.Unmarshal([]byte(raw), &v); err == nil {
				c.value.Store(&v)
				return
			}
		}
	}

	v, err := c.opts.Loader(ctx)
	if err != nil {
		logger.Warn("cache: warm up failed, Get will fall back to synchronous Loader calls until the first successful refresh",
			zap.String("key", c.opts.Key), zap.Error(err))
		return
	}
	c.value.Store(&v)
	c.writeToRedis(ctx, v)
}

func (c *Cache[T]) refreshSourceToRedis(ctx context.Context) error {
	c.applyJitter(ctx)

	v, err := c.opts.Loader(ctx)
	if err != nil {
		return err
	}
	c.writeToRedis(ctx, v)
	return nil
}

func (c *Cache[T]) refreshRedisToMemory(ctx context.Context) error {
	c.applyJitter(ctx)

	if client, ok := c.opts.Redis(); ok {
		if raw, err := client.Get(ctx, c.opts.Key).Result(); err == nil {
			var v T
			if err := json.Unmarshal([]byte(raw), &v); err == nil {
				c.value.Store(&v)
				return nil
			}
		}
	}

	v, err := c.opts.Loader(ctx)
	if err != nil {
		return err
	}
	c.value.Store(&v)
	return nil
}

func (c *Cache[T]) writeToRedis(ctx context.Context, v T) {
	client, ok := c.opts.Redis()
	if !ok {
		return
	}
	payload, err := json.Marshal(v)
	if err != nil {
		logger.Error("cache: marshal value failed", zap.String("key", c.opts.Key), zap.Error(err))
		alert.Notify(ctx, alert.Event{Scope: "cache", Name: c.opts.Key, Message: "marshal failed", Err: err})
		return
	}
	if err := client.Set(ctx, c.opts.Key, string(payload), c.opts.RedisTTL).Err(); err != nil {
		logger.Warn("cache: write to redis failed", zap.String("key", c.opts.Key), zap.Error(err))
	}
}

func (c *Cache[T]) applyJitter(ctx context.Context) {
	if c.opts.Jitter <= 0 {
		return
	}
	d := time.Duration(rand.Int63n(int64(c.opts.Jitter)))
	select {
	case <-time.After(d):
	case <-ctx.Done():
	}
}

func everySchedule(d time.Duration) string {
	if d < time.Second {
		d = time.Second
	}
	return "@every " + d.String()
}
