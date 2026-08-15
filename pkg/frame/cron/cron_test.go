package cron

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// TestComponentStartRejectsIncompleteTask 验证任务缺 Name/Schedule/Run 时 Start 返回 error。
func TestComponentStartRejectsIncompleteTask(t *testing.T) {
	cases := []Task{
		{Schedule: "@every 1s", Run: func(ctx context.Context) error { return nil }}, // 缺 Name
		{Name: "t", Run: func(ctx context.Context) error { return nil }},             // 缺 Schedule
		{Name: "t", Schedule: "@every 1s"},                                           // 缺 Run
	}
	for _, task := range cases {
		c := NewComponent("test", task)
		if err := c.Start(context.Background()); err == nil {
			t.Fatalf("配置不完整的任务 %+v 应该让 Start 返回 error", task)
		}
	}
}

// TestComponentStartRejectsDuplicateTaskName 验证重复任务名时 Start 返回 error。
func TestComponentStartRejectsDuplicateTaskName(t *testing.T) {
	run := func(ctx context.Context) error { return nil }
	c := NewComponent("test",
		Task{Name: "dup", Schedule: "@every 1s", Run: run},
		Task{Name: "dup", Schedule: "@every 2s", Run: run},
	)
	if err := c.Start(context.Background()); err == nil {
		t.Fatal("重复的任务名应该让 Start 返回 error")
	}
}

// TestComponentStartRejectsInvalidSchedule 验证非法 cron 表达式时 Start 返回 error。
func TestComponentStartRejectsInvalidSchedule(t *testing.T) {
	c := NewComponent("test", Task{Name: "bad", Schedule: "not a cron expr", Run: func(ctx context.Context) error { return nil }})
	if err := c.Start(context.Background()); err == nil {
		t.Fatal("非法的 cron 表达式应该让 Start 返回 error")
	}
}

// TestComponentTasksReturnsNameAndSchedule 验证 Tasks() 只返回 Name/Schedule，且无需 Start。
func TestComponentTasksReturnsNameAndSchedule(t *testing.T) {
	c := NewComponent("test",
		Task{Name: "a", Schedule: "@every 1s", Run: func(ctx context.Context) error { return nil }},
		Task{Name: "b", Schedule: "0 * * * * *", Run: func(ctx context.Context) error { return nil }},
	)

	infos := c.Tasks()
	if len(infos) != 2 {
		t.Fatalf("len(infos) = %d, want 2", len(infos))
	}
	if infos[0] != (TaskInfo{Name: "a", Schedule: "@every 1s"}) {
		t.Fatalf("infos[0] = %+v, 不符合预期", infos[0])
	}
	if infos[1] != (TaskInfo{Name: "b", Schedule: "0 * * * * *"}) {
		t.Fatalf("infos[1] = %+v, 不符合预期", infos[1])
	}
}

// TestComponentWithNoTasksStartsAndStopsCleanly 验证空任务列表时 Start/Stop 是安全 no-op。
func TestComponentWithNoTasksStartsAndStopsCleanly(t *testing.T) {
	c := NewComponent("test")
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("空任务列表 Start 不应该报错: %v", err)
	}
	if err := c.Stop(context.Background()); err != nil {
		t.Fatalf("空任务列表 Stop 不应该报错: %v", err)
	}
}

// minSchedule 是 go-cron 允许的最短 @every 间隔，单测统一用这个值。
const minSchedule = "@every 1s"

// TestComponentExecutesTaskOnSchedule 验证任务会按调度真正执行。
func TestComponentExecutesTaskOnSchedule(t *testing.T) {
	var calls atomic.Int32
	c := NewComponent("test", Task{
		Name:     "tick",
		Schedule: minSchedule,
		Run:      func(ctx context.Context) error { calls.Add(1); return nil },
	})

	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start 失败: %v", err)
	}
	defer c.Stop(context.Background())

	deadline := time.Now().Add(4500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if calls.Load() >= 3 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("4.5s 内应该至少执行 3 次（调度间隔 1s），实际 %d 次", calls.Load())
}

// TestComponentSupportsStandardWildcardSchedule 验证标准 6 段 cron 表达式能被解析并调度。
func TestComponentSupportsStandardWildcardSchedule(t *testing.T) {
	var calls atomic.Int32
	c := NewComponent("test", Task{
		Name:     "every-second",
		Schedule: "* * * * * *", // 标准 6 段通配符：每秒触发，不是 "@every 1s" 简写
		Run:      func(ctx context.Context) error { calls.Add(1); return nil },
	})

	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("标准通配符表达式应该能被正常解析和调度，Start 失败: %v", err)
	}
	defer c.Stop(context.Background())

	deadline := time.Now().Add(3500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if calls.Load() >= 3 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("3.5s 内应该至少触发 3 次（每秒一次），实际 %d 次", calls.Load())
}

// TestComponentSkipsOverlappingRuns 验证同一任务不会并发重叠执行。
func TestComponentSkipsOverlappingRuns(t *testing.T) {
	var running atomic.Int32
	var maxConcurrent atomic.Int32
	var totalRuns atomic.Int32

	c := NewComponent("test", Task{
		Name:     "slow",
		Schedule: minSchedule,
		Run: func(ctx context.Context) error {
			cur := running.Add(1)
			for {
				m := maxConcurrent.Load()
				if cur <= m || maxConcurrent.CompareAndSwap(m, cur) {
					break
				}
			}
			totalRuns.Add(1)
			time.Sleep(1300 * time.Millisecond) // 明显比 1s 的调度间隔长，制造重叠触发的机会
			running.Add(-1)
			return nil
		},
	})

	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start 失败: %v", err)
	}
	time.Sleep(4 * time.Second)
	if err := c.Stop(context.Background()); err != nil {
		t.Fatalf("Stop 失败: %v", err)
	}

	if maxConcurrent.Load() > 1 {
		t.Fatalf("SkipIfStillRunning 应该保证同一个任务不会并发执行，实际同时跑到了 %d 次", maxConcurrent.Load())
	}
	if totalRuns.Load() == 0 {
		t.Fatal("任务应该至少被执行过一次")
	}
}

// TestComponentRecoversFromPanic 验证单个任务 panic 后调度器仍会继续触发后续轮次。
func TestComponentRecoversFromPanic(t *testing.T) {
	var calls atomic.Int32
	c := NewComponent("test", Task{
		Name:     "panicky",
		Schedule: minSchedule,
		Run: func(ctx context.Context) error {
			n := calls.Add(1)
			if n == 1 {
				panic("boom")
			}
			return nil
		},
	})

	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start 失败: %v", err)
	}
	defer c.Stop(context.Background())

	deadline := time.Now().Add(4500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if calls.Load() >= 3 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("第一次执行 panic 之后，调度器应该继续正常触发后续轮次（4.5s 内应至少 3 次），实际总共只执行了 %d 次", calls.Load())
}

// TestComponentStopWaitsForRunningTask 验证 Stop 会在 ctx 超时前等待正在运行的任务结束。
func TestComponentStopWaitsForRunningTask(t *testing.T) {
	var finished atomic.Bool
	c := NewComponent("test", Task{
		Name:     "one-shot",
		Schedule: minSchedule,
		Run: func(ctx context.Context) error {
			time.Sleep(300 * time.Millisecond)
			finished.Store(true)
			return nil
		},
	})

	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start 失败: %v", err)
	}
	time.Sleep(1100 * time.Millisecond) // 等第一次调度触发（间隔 1s）并确保任务已经开始跑

	stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := c.Stop(stopCtx); err != nil {
		t.Fatalf("Stop 失败: %v", err)
	}
	if !finished.Load() {
		t.Fatal("Stop 应该等正在运行的任务跑完，但任务并没有跑完")
	}
}

// TestComponentStopRespectsDeadline 验证任务超过 ctx 超时后 Stop 会按时返回。
func TestComponentStopRespectsDeadline(t *testing.T) {
	c := NewComponent("test", Task{
		Name:     "too-slow",
		Schedule: minSchedule,
		Run: func(ctx context.Context) error {
			time.Sleep(3 * time.Second) // 远超下面 ctx 的超时预算
			return nil
		},
	})

	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start 失败: %v", err)
	}
	time.Sleep(1100 * time.Millisecond) // 等第一次调度触发（间隔 1s）并确保任务已经开始跑

	stopCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	if err := c.Stop(stopCtx); err != nil {
		t.Fatalf("Stop 失败: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("Stop 应该在 ctx 到期（约 100ms）后就返回，实际等了 %v", elapsed)
	}
}

// TestComponentCancelsTaskContextOnStop 验证 Stop 会取消正在运行任务的 ctx。
func TestComponentCancelsTaskContextOnStop(t *testing.T) {
	var canceled atomic.Bool
	started := make(chan struct{})

	c := NewComponent("test", Task{
		Name:     "ctx-aware",
		Schedule: minSchedule,
		Run: func(ctx context.Context) error {
			select {
			case started <- struct{}{}:
			default:
			}
			select {
			case <-ctx.Done():
				canceled.Store(true)
			case <-time.After(5 * time.Second):
			}
			return nil
		},
	})

	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start 失败: %v", err)
	}

	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("任务应该已经开始运行（调度间隔 1s）")
	}

	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := c.Stop(stopCtx); err != nil {
		t.Fatalf("Stop 失败: %v", err)
	}
	if !canceled.Load() {
		t.Fatal("Stop 应该取消正在运行任务的 ctx，但任务没有观察到取消信号")
	}
}

// TestComponentRecordsMetrics 验证成功/失败次数会被记录，且采集器能注册进 Registry。
func TestComponentRecordsMetrics(t *testing.T) {
	c := NewComponent("test",
		Task{Name: "ok-task", Schedule: minSchedule, Run: func(ctx context.Context) error { return nil }},
		Task{Name: "bad-task", Schedule: minSchedule, Run: func(ctx context.Context) error { return errors.New("boom") }},
	)

	reg := prometheus.NewRegistry()
	if err := reg.Register(c.runsTotal); err != nil {
		t.Fatalf("注册 runsTotal 采集器失败: %v", err)
	}
	if err := reg.Register(c.duration); err != nil {
		t.Fatalf("注册 duration 采集器失败: %v", err)
	}
	if len(c.Collectors()) != 2 {
		t.Fatalf("Collectors() 应该返回 2 个采集器，实际 %d 个", len(c.Collectors()))
	}

	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start 失败: %v", err)
	}
	defer c.Stop(context.Background())
	time.Sleep(2200 * time.Millisecond) // 等两轮调度触发（间隔 1s），确保成功/失败各至少发生一次

	metrics, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather 失败: %v", err)
	}

	var sawSuccess, sawFailure bool
	for _, m := range metrics {
		if m.GetName() != "cron_task_runs_total" {
			continue
		}
		for _, metric := range m.GetMetric() {
			if hasLabel(metric, "task", "ok-task") && hasLabel(metric, "status", "success") && metric.GetCounter().GetValue() > 0 {
				sawSuccess = true
			}
			if hasLabel(metric, "task", "bad-task") && hasLabel(metric, "status", "failure") && metric.GetCounter().GetValue() > 0 {
				sawFailure = true
			}
		}
	}
	if !sawSuccess {
		t.Fatal("应该观察到 ok-task 的 success 计数 > 0")
	}
	if !sawFailure {
		t.Fatal("应该观察到 bad-task 的 failure 计数 > 0")
	}
}

// TestComponentRecordsPanicAsDistinctMetricStatus 验证 panic 记为 status=panic，不会从指标里消失。
func TestComponentRecordsPanicAsDistinctMetricStatus(t *testing.T) {
	c := NewComponent("test", Task{
		Name:     "panic-metric",
		Schedule: minSchedule,
		Run:      func(ctx context.Context) error { panic("always panics") },
	})

	reg := prometheus.NewRegistry()
	if err := reg.Register(c.runsTotal); err != nil {
		t.Fatalf("注册 runsTotal 采集器失败: %v", err)
	}

	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start 失败: %v", err)
	}
	defer c.Stop(context.Background())
	time.Sleep(1300 * time.Millisecond) // 等第一次调度触发（间隔 1s）

	metrics, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather 失败: %v", err)
	}

	var sawPanic bool
	for _, m := range metrics {
		if m.GetName() != "cron_task_runs_total" {
			continue
		}
		for _, metric := range m.GetMetric() {
			if hasLabel(metric, "task", "panic-metric") && hasLabel(metric, "status", "panic") && metric.GetCounter().GetValue() > 0 {
				sawPanic = true
			}
		}
	}
	if !sawPanic {
		t.Fatal("panic 应该被记录为一次 status=panic 的执行，而不是在指标里彻底消失")
	}
}

func hasLabel(m *dto.Metric, name, value string) bool {
	for _, l := range m.GetLabel() {
		if l.GetName() == name && l.GetValue() == value {
			return true
		}
	}
	return false
}

// TestMultipleComponentsWithDifferentNamesCanCoexistInSameRegistry 验证两个不同 name
// 的 Component 能一起把指标注册进同一个 Registry，不会因为同名冲突。
func TestMultipleComponentsWithDifferentNamesCanCoexistInSameRegistry(t *testing.T) {
	c1 := NewComponent("component-a")
	c2 := NewComponent("component-b")

	reg := prometheus.NewRegistry()
	for _, collector := range c1.Collectors() {
		if err := reg.Register(collector); err != nil {
			t.Fatalf("注册第一个 Component 的指标失败: %v", err)
		}
	}
	for _, collector := range c2.Collectors() {
		if err := reg.Register(collector); err != nil {
			t.Fatalf("注册第二个 Component 的指标失败（说明 name 没能区分开两个同名指标）: %v", err)
		}
	}
}

// TestToZapFieldsHandlesOddKeysAndValues 验证奇数个 key/value 时 toZapFields 不 panic。
func TestToZapFieldsHandlesOddKeysAndValues(t *testing.T) {
	fields := toZapFields([]any{"k1", "v1", "dangling"})
	if len(fields) != 2 {
		t.Fatalf("len(fields) = %d, want 2", len(fields))
	}

	var la logAdapter
	// 只验证不 panic；实际日志内容不在这个单测的关注范围内。
	la.Info("test message", "k1", "v1", "dangling")
	la.Error(errors.New("boom"), "test error", "k1", "v1")
}
