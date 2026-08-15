// Package cron 是本示例的定时任务接线层：登记任务名、调度表达式和要调用的函数。
package cron

import (
	"context"
	"time"

	"github.com/boloc/go-frame-server/internal/example/repository"
	"github.com/boloc/go-frame-server/pkg/frame/cron"
	"github.com/boloc/go-frame-server/pkg/frame/webx"
	"github.com/boloc/go-frame-server/pkg/logger"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Component 本示例的定时任务单例；新增任务往这里加 Task，不要再建第二个 Component。
// name 传 "example"：区分 pkg/frame/refreshcache 内部各自持有的 cron.Component
// （用各自的 Key 当 name），避免 Prometheus 指标同名冲突，见 cron.NewComponent 的
// 文档注释。
var Component = cron.NewComponent("example",
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
		Name:     "report-low-stock",
		Schedule: "0 */5 * * * *", // 每 5 分钟检查一次库存
		Run:      repository.DefaultOrderRepository().ReportLowStock,
	},
)

func heartbeat(ctx context.Context) error {
	logger.Info("cron: heartbeat", zap.Time("at", time.Now()))
	return nil
}

func minuteMarker(ctx context.Context) error {
	logger.Info("cron: minute marker", zap.Time("at", time.Now()))
	return nil
}

// TaskListHandler 返回已注册任务（名称+调度表达式）。
// GET /api/cron/tasks
func TaskListHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		webx.Success(c, Component.Tasks())
	}
}
