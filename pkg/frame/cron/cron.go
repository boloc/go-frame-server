// Package cron 提供定时任务组件：任务未完成时跳过下次触发，记录执行指标，随 Frame 优雅关闭。
package cron

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/boloc/go-frame-server/pkg/alert"
	"github.com/boloc/go-frame-server/pkg/logger"
	gocron "github.com/netresearch/go-cron"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

// Task 一个定时任务。Schedule 支持 6 段 cron 表达式或 "@every 10m"。
type Task struct {
	// Name 任务名，同一 Component 内必须唯一。
	Name string
	// Schedule cron 表达式，必须非空。
	Schedule string
	// Run 任务逻辑。Stop 时会取消 ctx，耗时操作应把 ctx 传下去。
	Run func(ctx context.Context) error
}

// TaskInfo 任务的可展示信息，不含 Run。
type TaskInfo struct {
	Name     string
	Schedule string
}

// Component 定时任务调度组件，实现 frame.Component（Start(ctx)/Stop(ctx)）。
type Component struct {
	tasks []Task

	engine *gocron.Cron

	runsTotal *prometheus.CounterVec
	duration  *prometheus.HistogramVec
}

// NewComponent 创建定时任务组件。任务列表在构造时一次性传入。
//
// name 是这个 Component 在进程内唯一的标识，作为 ConstLabels 挂在
// cron_task_runs_total/cron_task_duration_seconds 上——同一进程有多个 Component 时
// （比如 pkg/frame/refreshcache.Cache 内部各自持有一个），name 不同才能避免指标
// 因为完全同名同 label 在 MustRegister 时冲突 panic。
func NewComponent(name string, tasks ...Task) *Component {
	constLabels := prometheus.Labels{"scheduler": name}
	return &Component{
		tasks: tasks,
		runsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name:        "cron_task_runs_total",
			Help:        "定时任务执行次数，按 task 名称和 status(success/failure) 分类",
			ConstLabels: constLabels,
		}, []string{"task", "status"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:        "cron_task_duration_seconds",
			Help:        "定时任务单次执行耗时（秒）",
			ConstLabels: constLabels,
			Buckets:     prometheus.DefBuckets,
		}, []string{"task"}),
	}
}

// Tasks 返回已注册任务的名称和调度表达式，不暴露 Run。
func (c *Component) Tasks() []TaskInfo {
	infos := make([]TaskInfo, len(c.tasks))
	for i, t := range c.tasks {
		infos[i] = TaskInfo{Name: t.Name, Schedule: t.Schedule}
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

func (c *Component) wrap(t Task) func(ctx context.Context) {
	return func(ctx context.Context) {
		start := time.Now()
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
			logger.Info("cron: task finished",
				zap.String("task", t.Name), zap.Duration("elapsed", elapsed))
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
