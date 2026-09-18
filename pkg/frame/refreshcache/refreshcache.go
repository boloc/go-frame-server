// Package refreshcache 提供双层定时刷新只读缓存：数据源 → Redis → 进程内存。
// Redis 不可用时内存层回退到数据源。适合读多写少、允许有界延迟的数据。
package refreshcache

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand"
	"sync/atomic"
	"time"

	"github.com/boloc/go-frame-server/pkg/alert"
	"github.com/boloc/go-frame-server/pkg/frame"
	"github.com/boloc/go-frame-server/pkg/frame/cron"
	"github.com/boloc/go-frame-server/pkg/frame/rediskey"
	"github.com/boloc/go-frame-server/pkg/logger"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
)

// Loader 从数据源加载最新数据。
type Loader[T any] func(ctx context.Context) (T, error)

// Options 缓存配置。Key/Loader/RedisInterval/MemoryInterval 必须显式设置。
type Options[T any] struct {
	// Key 是这个缓存实例在进程内的标识：内部 cron.Component 的 name、两条刷新任务的
	// 名字后缀、singleflight 的合并键和日志字段都用它。它不是 Redis key 本身，
	// 写进 Redis 时前面会加上命名空间和缓存段，见 Cache.RedisKey。
	Key string
	// Loader 从数据源加载最新数据。
	Loader Loader[T]
	// RedisInterval 刷新间隔。
	RedisInterval time.Duration
	// MemoryInterval 内存刷新间隔。
	MemoryInterval time.Duration
	// RedisTTL 必须明显大于 RedisInterval，默认 RedisInterval * 5。
	RedisTTL time.Duration
	// Jitter 刷新前的随机抖动上限，多实例部署时用来错开刷新时机。默认 0，不抖动。
	Jitter time.Duration
	// Redis 获取 Redis 客户端。
	Redis func() (redis.Cmdable, bool)
}

// Cache 双层刷新只读缓存，实现 frame.Component（Start(ctx)/Stop(ctx)）。
type Cache[T any] struct {
	opts Options[T]
	cron *cron.Component

	value atomic.Pointer[T]

	// sf 合并 Get() 在内存未就绪时的并发 Loader 调用，避免这个短暂窗口内的多个并发
	// 请求各自触发一次 Loader，在数据源本身脆弱时把"缓存还没预热好"变成一次雪崩。
	// 零值可直接用，不需要初始化。
	sf singleflight.Group
}

// New 创建双层刷新只读缓存。必填项未设置时 Start 返回 error，New 本身不检查、不 panic。
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
		// 从数据源刷新到 Redis
		cron.Task{
			Name:     "cache-refresh-source-to-redis:" + opts.Key,
			Schedule: everySchedule(opts.RedisInterval),
			Run:      c.refreshSourceToRedis,
		},
		// 从 Redis 刷新到内存
		cron.Task{
			Name:     "cache-refresh-redis-to-memory:" + opts.Key,
			Schedule: everySchedule(opts.MemoryInterval),
			Run:      c.refreshRedisToMemory,
		},
	)
	return c
}

// RedisKey 返回缓存值实际落在 Redis 上的 key：命名空间 + rediskey.SegCache + Options.Key。
func (c *Cache[T]) RedisKey() string {
	return rediskey.Prefix(rediskey.SegCache) + c.opts.Key
}

// Get 优先读内存；内存为空时同步调用 Loader，不写回缓存。ok 为 false 表示两边都拿不到。
//
// 内存为空的这个窗口用 singleflight 按 opts.Key 合并并发调用：同一时刻只有一个 goroutine
// 真正执行 Loader，其它并发的 Get 调用等它返回后共享同一个结果，不会各自打一次数据源。
// Loader 用 context.WithoutCancel(ctx) 而不是直接传调用方的 ctx：如果某个请求的 ctx 先被
// 取消（客户端断开连接），不应该连带取消正在为其它并发请求服务的这一次 Loader 调用。
func (c *Cache[T]) Get(ctx context.Context) (T, bool) {
	if v := c.value.Load(); v != nil {
		return *v, true
	}

	loaderCtx := context.WithoutCancel(ctx)
	v, err, _ := c.sf.Do(c.opts.Key, func() (any, error) {
		return c.opts.Loader(loaderCtx)
	})
	if err != nil {
		var zero T
		return zero, false
	}
	return v.(T), true
}

// Start 校验配置、同步预热，然后启动两条刷新任务。
func (c *Cache[T]) Start(ctx context.Context) error {
	if c.opts.Key == "" {
		return errors.New("refreshcache: Options.Key 必须非空")
	}
	if c.opts.Loader == nil {
		return errors.New("refreshcache: Options.Loader 必须非空")
	}
	if c.opts.RedisInterval <= 0 {
		return errors.New("refreshcache: Options.RedisInterval 必须大于 0")
	}
	if c.opts.MemoryInterval <= 0 {
		return errors.New("refreshcache: Options.MemoryInterval 必须大于 0")
	}
	// 预热和刷新都要拼 Redis key。在这里挡住，避免"应用忘了注入命名空间"变成
	// warmUp 里的运行期 panic。
	if !rediskey.IsSet() {
		return errors.New("refreshcache: Redis 命名空间尚未设置，" +
			"应用需要在注册组件之前调用 rediskey.SetNamespace")
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
		if raw, err := client.Get(ctx, c.RedisKey()).Result(); err == nil {
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
	if err := c.applyJitter(ctx); err != nil {
		return err
	}

	v, err := c.opts.Loader(ctx)
	if err != nil {
		return err
	}
	c.writeToRedis(ctx, v)
	return nil
}

func (c *Cache[T]) refreshRedisToMemory(ctx context.Context) error {
	if err := c.applyJitter(ctx); err != nil {
		return err
	}

	if client, ok := c.opts.Redis(); ok {
		if raw, err := client.Get(ctx, c.RedisKey()).Result(); err == nil {
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
	if err := client.Set(ctx, c.RedisKey(), string(payload), c.opts.RedisTTL).Err(); err != nil {
		logger.Warn("cache: write to redis failed",
			zap.String("key", c.opts.Key), zap.String("redis_key", c.RedisKey()), zap.Error(err))
	}
}

func (c *Cache[T]) applyJitter(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if c.opts.Jitter <= 0 {
		return nil
	}
	d := time.Duration(rand.Int63n(int64(c.opts.Jitter)))
	select {
	case <-time.After(d):
		return ctx.Err()
	case <-ctx.Done():
		return ctx.Err()
	}
}

func everySchedule(d time.Duration) string {
	if d < time.Second {
		d = time.Second
	}
	return "@every " + d.String()
}
