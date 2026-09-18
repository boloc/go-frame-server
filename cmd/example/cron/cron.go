// Package cron 是本示例的定时任务接线层：登记任务名、调度表达式和要调用的函数。
package cron

import (
	"context"
	"time"

	"github.com/boloc/go-frame-server/v2/internal/example/repository"
	"github.com/boloc/go-frame-server/v2/pkg/frame/cron"
	"github.com/boloc/go-frame-server/v2/pkg/frame/webx"
	"github.com/boloc/go-frame-server/v2/pkg/logger"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Component 本示例的定时任务单例；新增任务往这里加 Task，不要再建第二个 Component。
//
// 第一个参数 "crontab-tasks" 是调度器名，进程内必须唯一，它有两个用途：
//   - Prometheus 指标上的 scheduler 标签（cron_task_runs_total/cron_task_duration_seconds）；
//   - Exclusive 任务锁 key 的一段，见 cron.Component.LockKey。
//
// 必须区别于 pkg/frame/refreshcache 内部各自持有的 cron.Component（用各自的 Options.Key
// 当调度器名），否则 Prometheus 指标同名冲突，见 cron.NewComponent 的文档注释。
var Component = cron.NewComponent("crontab-tasks",
	cron.Task{
		Name:     "heartbeat",
		Schedule: "@every 30s",
		Run:      heartbeat,
	},
	cron.Task{
		Name:     "minute-marker",
		Schedule: "0 * * * * *", // 标准 6 段通配符：每分钟第 0 秒触发一次
		Run:      minuteMarker,
	},
	cron.Task{
		Name:      "report-low-stock",
		Schedule:  "0 */5 * * * *", // 每 5 分钟检查一次库存
		Exclusive: true,            // 多副本时抢 Redis 锁（key 见 Component.LockKey / GET /api/cron/tasks）；抢不到 skipped，不告警
		LockTTL:   2 * time.Minute, // 必须大于单次最长执行时间；没有自动续租
		Run:       repository.DefaultOrderRepository().ReportLowStock,
	},
	// heartbeat / minute-marker 保持非互斥，每个副本都会执行，作为 Exclusive 的对照。
	// Exclusive 任务 Redis 不可用时宁可漏跑一轮（skipped + Warn + alert），也不要多副本同时跑。
)

func heartbeat(ctx context.Context) error {
	logger.Info("cron: heartbeat", zap.Time("at", time.Now()))
	return nil
}

func minuteMarker(ctx context.Context) error {
	logger.Info("cron: minute marker", zap.Time("at", time.Now()))
	return nil
}

// TaskListHandler 返回已注册任务（名称、调度表达式、是否 Exclusive、实际的锁 key）。
//
//	GET /api/cron/tasks
//	curl localhost:10006/api/cron/tasks
func TaskListHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		webx.Success(c, Component.Tasks())
	}
}
