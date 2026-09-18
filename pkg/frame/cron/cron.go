// Package cron 提供定时任务组件：任务未完成时跳过下次触发，记录执行指标，随 Frame 优雅关闭。
// Exclusive 任务通过 Redis 分布式锁保证多副本下同一时刻只有一个副本执行。
package cron

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/boloc/go-frame-server/pkg/alert"
	"github.com/boloc/go-frame-server/pkg/frame"
	"github.com/boloc/go-frame-server/pkg/frame/rediskey"
	"github.com/boloc/go-frame-server/pkg/logger"
	gocron "github.com/netresearch/go-cron"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// DefaultLockTTL 是 Task.LockTTL 为 0 时的默认租约。没有自动续租。
const DefaultLockTTL = 10 * time.Minute

// releaseLockScript 只删自己持有的锁：value 对得上才 DEL，避免误删后抢到的副本的锁。
// 走 redis.NewScript.Run：先 EVALSHA，遇到 NOSCRIPT 再退回 EVAL。
var releaseLockScript = redis.NewScript(`if redis.call("GET",KEYS[1])==ARGV[1] then return redis.call("DEL",KEYS[1]) end return 0`)

// Task 一个定时任务。Schedule 支持 6 段 cron 表达式或 "@every 10m"。
type Task struct {
	// Name 任务名，同一 Component 内必须唯一。
	Name string
	// Schedule cron 表达式，必须非空。
	Schedule string
	// Run 任务逻辑。Stop 时会取消 ctx，耗时操作应把 ctx 传下去。
	Run func(ctx context.Context) error

	// Exclusive 为 true 时，执行前先抢 Redis 锁（SET NX PX），抢不到就跳过本次
	// （status="skipped"，Info 日志，不告警）。Exclusive 的任务通常不幂等：Redis
	// 不可用（拿不到 client 或 SET 报错）时宁可漏跑一轮（skipped + Warn + alert），
	// 也不要多副本同时跑。
	Exclusive bool
	// LockTTL 锁的租约时长，必须大于任务单次最长执行时间；为 0 时默认 DefaultLockTTL。
	// 没有自动续租，任务跑得比 LockTTL 久会有第二个副本并发进来。
	LockTTL time.Duration
}

// TaskInfo 任务的可展示信息，不含 Run。
type TaskInfo struct {
	Name      string
	Schedule  string
	Exclusive bool
	// LockKey 是这个任务实际用的分布式锁 key；非 Exclusive 任务不加锁，为空。
	LockKey string
}

// Option 配置 cron.Component 的函数式选项。
type Option func(*Component)

// WithRedis 覆盖默认的 Redis 获取函数。fn 为 nil 时忽略。
// 默认是 frame.TryGetRedisCmdable。
func WithRedis(fn func() (redis.Cmdable, bool)) Option {
	return func(c *Component) {
		if fn != nil {
			c.redis = fn
		}
	}
}

// Component 定时任务调度组件，实现 frame.Component（Start(ctx)/Stop(ctx)）。
type Component struct {
	name  string
	tasks []Task
	redis func() (redis.Cmdable, bool)

	engine *gocron.Cron

	runsTotal *prometheus.CounterVec
	duration  *prometheus.HistogramVec
}

// NewComponent 创建定时任务组件。任务列表在构造时一次性传入。
// Exclusive 任务默认用 frame.TryGetRedisCmdable 取 Redis；测试或自定义客户端用
// NewComponentWithOptions + WithRedis。
//
// name 是这个 Component 在进程内唯一的标识，作为 ConstLabels 挂在
// cron_task_runs_total/cron_task_duration_seconds 上——同一进程有多个 Component 时
// （比如 pkg/frame/refreshcache.Cache 内部各自持有一个），name 不同才能避免指标
// 因为完全同名同 label 在 MustRegister 时冲突 panic。
func NewComponent(name string, tasks ...Task) *Component {
	return NewComponentWithOptions(name, nil, tasks...)
}

// NewComponentWithOptions 与 NewComponent 相同，并接受 Component 级选项（如 WithRedis）。
func NewComponentWithOptions(name string, opts []Option, tasks ...Task) *Component {
	constLabels := prometheus.Labels{"scheduler": name}
	c := &Component{
		name:  name,
		tasks: tasks,
		redis: frame.TryGetRedisCmdable,
		runsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name:        "cron_task_runs_total",
			Help:        "定时任务执行次数，按 task 名称和 status(success/failure/canceled/panic/skipped) 分类",
			ConstLabels: constLabels,
		}, []string{"task", "status"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:        "cron_task_duration_seconds",
			Help:        "定时任务单次执行耗时（秒）",
			ConstLabels: constLabels,
			Buckets:     prometheus.DefBuckets,
		}, []string{"task"}),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(c)
		}
	}
	return c
}

// Name 返回这个调度器的名字，也就是 NewComponent 的第一个参数：它既是 Prometheus 指标上
// 的 scheduler 标签，也是 Exclusive 任务锁 key 的一段。
func (c *Component) Name() string {
	return c.name
}

// LockKey 返回任务名对应的分布式锁 key：命名空间 + rediskey.SegCronLock + 调度器名 +
// ":" + 任务名。只有 Exclusive 任务会真的用到它；非 Exclusive 任务不加锁。
func (c *Component) LockKey(taskName string) string {
	return rediskey.Prefix(rediskey.SegCronLock) + c.name + ":" + taskName
}

// Tasks 返回已注册任务的名称、调度表达式和加锁信息，不暴露 Run。
func (c *Component) Tasks() []TaskInfo {
	infos := make([]TaskInfo, len(c.tasks))
	for i, t := range c.tasks {
		infos[i] = TaskInfo{Name: t.Name, Schedule: t.Schedule, Exclusive: t.Exclusive}
		if t.Exclusive {
			infos[i].LockKey = c.LockKey(t.Name)
		}
	}
	return infos
}

// Collectors 返回需要注册到 Prometheus 的采集器。
func (c *Component) Collectors() []prometheus.Collector {
	return []prometheus.Collector{c.runsTotal, c.duration}
}

// Start 校验任务配置并启动调度器。配置不完整或任务名重复时返回 error。
func (c *Component) Start(_ context.Context) error {
	if len(c.tasks) == 0 {
		logger.Info("cron: no tasks registered, scheduler not started")
		return nil
	}

	// 创建一个定时任务调度器
	engine := gocron.New(gocron.WithSeconds(), gocron.WithChain(
		gocron.Recover(logAdapter{}),
		gocron.SkipIfStillRunning(logAdapter{}),
	))

	// 校验任务配置并注册任务
	seen := make(map[string]bool, len(c.tasks))
	for _, task := range c.tasks {
		if task.Name == "" || task.Schedule == "" || task.Run == nil {
			return fmt.Errorf("cron: 任务配置不完整（Name/Schedule/Run 均不能为空），任务: %+v", task)
		}
		if task.Exclusive && task.LockTTL < 0 {
			return fmt.Errorf("cron: 任务 %q 的 LockTTL 不能为负数", task.Name)
		}
		// Exclusive 任务的锁 key 依赖 Redis 命名空间。在这里挡住，否则"应用忘了注入"
		// 会拖到任务第一次触发时才 panic，那已经不是启动期了。
		if task.Exclusive && !rediskey.IsSet() {
			return fmt.Errorf("cron: 任务 %q 是 Exclusive，但 Redis 命名空间尚未设置，"+
				"应用需要在注册组件之前调用 rediskey.SetNamespace", task.Name)
		}
		if seen[task.Name] {
			return fmt.Errorf("cron: 任务名 %q 重复注册", task.Name)
		}
		seen[task.Name] = true

		job := gocron.FuncJobWithContext(c.wrap(task))
		if _, err := engine.AddJob(task.Schedule, job, gocron.WithName(task.Name)); err != nil {
			return fmt.Errorf("cron: 注册任务 %q 失败（Schedule=%q）: %w", task.Name, task.Schedule, err)
		}
	}

	c.engine = engine
	c.engine.Start()
	logger.Info("cron: scheduler started", zap.Int("tasks", len(c.tasks)))
	return nil
}

// wrap 包装任务函数，添加日志、告警、指标采集等功能。
func (c *Component) wrap(t Task) func(ctx context.Context) {
	return func(ctx context.Context) {
		start := time.Now()

		if t.Exclusive {
			token, acquired := c.acquireExclusive(ctx, t)
			if !acquired {
				elapsed := time.Since(start)
				c.runsTotal.WithLabelValues(t.Name, "skipped").Inc()
				c.duration.WithLabelValues(t.Name).Observe(elapsed.Seconds())
				return
			}
			defer c.releaseExclusive(ctx, t, token)
		}

		defer func() {
			if r := recover(); r != nil {
				c.runsTotal.WithLabelValues(t.Name, "panic").Inc()
				c.duration.WithLabelValues(t.Name).Observe(time.Since(start).Seconds())
				alert.Notify(ctx, alert.Event{
					Scope: "cron", Name: t.Name, Message: "task panicked",
					Fields: map[string]any{"panic": r},
				})
				panic(r)
			}
		}()

		err := t.Run(ctx)
		elapsed := time.Since(start)

		status := "success"
		switch {
		case err == nil:
			// 非特殊调试情况下正常执行不打印日志，不然会打印很多日志
			// logger.Info("cron: task finished",
			// 	zap.String("task", t.Name), zap.Duration("elapsed", elapsed))
		case errors.Is(err, context.Canceled):
			// 进程退出时 Stop 会取消任务 ctx，查库/刷缓存被掐断是正常收尾，不当失败、不告警。
			status = "canceled"
			logger.Info("cron: task canceled",
				zap.String("task", t.Name), zap.Duration("elapsed", elapsed))
		default:
			status = "failure"
			logger.Error("cron: task failed",
				zap.String("task", t.Name), zap.Duration("elapsed", elapsed), zap.Error(err))
			alert.Notify(ctx, alert.Event{
				Scope: "cron", Name: t.Name, Message: "task failed", Err: err,
				Fields: map[string]any{"elapsed": elapsed.String()},
			})
		}
		c.runsTotal.WithLabelValues(t.Name, status).Inc()
		c.duration.WithLabelValues(t.Name).Observe(elapsed.Seconds())
	}
}

func (c *Component) acquireExclusive(ctx context.Context, t Task) (token string, acquired bool) {
	ttl := t.LockTTL
	if ttl == 0 {
		ttl = DefaultLockTTL
	}

	if c.redis == nil {
		c.lockUnavailable(ctx, t, "redis getter is nil", nil)
		return "", false
	}
	client, ok := c.redis()
	if !ok || client == nil {
		c.lockUnavailable(ctx, t, "redis unavailable", nil)
		return "", false
	}

	token, err := newLockToken()
	if err != nil {
		c.lockUnavailable(ctx, t, "lock token generation failed", err)
		return "", false
	}

	claimed, err := client.SetNX(ctx, c.LockKey(t.Name), token, ttl).Result()
	if err != nil {
		c.lockUnavailable(ctx, t, "SET failed", err)
		return "", false
	}
	if !claimed {
		logger.Info("cron: exclusive lock not acquired, task skipped",
			zap.String("task", t.Name), zap.String("scheduler", c.name),
			zap.String("lock_key", c.LockKey(t.Name)))
		return "", false
	}
	return token, true
}

func (c *Component) releaseExclusive(ctx context.Context, t Task, token string) {
	if c.redis == nil {
		return
	}
	client, ok := c.redis()
	if !ok || client == nil {
		logger.Warn("cron: exclusive lock release skipped, redis unavailable",
			zap.String("task", t.Name))
		return
	}

	releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	if err := releaseLockScript.Run(releaseCtx, client, []string{c.LockKey(t.Name)}, token).Err(); err != nil {
		logger.Warn("cron: exclusive lock release failed",
			zap.String("task", t.Name), zap.Error(err))
	}
}

func (c *Component) lockUnavailable(ctx context.Context, t Task, reason string, err error) {
	logger.Warn("cron: exclusive lock unavailable, task skipped",
		zap.String("task", t.Name), zap.String("scheduler", c.name),
		zap.String("reason", reason), zap.Error(err))
	alert.Notify(ctx, alert.Event{
		Scope:   "cron",
		Name:    t.Name,
		Message: "exclusive lock unavailable, task skipped",
		Err:     err,
		Fields:  map[string]any{"reason": reason, "scheduler": c.name},
	})
}

func newLockToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Stop 停止调度器并等待正在运行的任务结束；超过 ctx 超时后不再等待。
func (c *Component) Stop(ctx context.Context) error {
	if c.engine == nil {
		return nil
	}

	stopped := c.engine.Stop()
	select {
	case <-stopped.Done():
		logger.Info("cron: scheduler stopped, all running tasks finished")
	case <-ctx.Done():
		logger.Warn("cron: shutdown deadline reached while some tasks are still running")
	}
	return nil
}

type logAdapter struct{}

func (logAdapter) Info(msg string, keysAndValues ...any) {
	logger.Info("cron: "+msg, toZapFields(keysAndValues)...)
}

func (logAdapter) Error(err error, msg string, keysAndValues ...any) {
	fields := append(toZapFields(keysAndValues), zap.Error(err))
	logger.Error("cron: "+msg, fields...)
}

func toZapFields(keysAndValues []any) []zap.Field {
	fields := make([]zap.Field, 0, len(keysAndValues)/2+1)
	for i := 0; i+1 < len(keysAndValues); i += 2 {
		key, ok := keysAndValues[i].(string)
		if !ok {
			key = fmt.Sprintf("%v", keysAndValues[i])
		}
		fields = append(fields, zap.Any(key, keysAndValues[i+1]))
	}
	if len(keysAndValues)%2 == 1 {
		fields = append(fields, zap.Any("extra", keysAndValues[len(keysAndValues)-1]))
	}
	return fields
}
