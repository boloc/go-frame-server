package route

import (
	"github.com/boloc/go-frame-server/cmd/example/cron"
	"github.com/boloc/go-frame-server/internal/example/handler"
	"github.com/gin-gonic/gin"
)

// registerCronRoutes 演示查看已注册任务，以及手动立即执行一次库存检查。
//
//	curl localhost:10006/api/cron/tasks
//	curl -X POST localhost:10006/api/cron/report-low-stock
func registerCronRoutes(r *gin.Engine) {
	r.GET("/api/cron/tasks", cron.TaskListHandler())
	r.POST("/api/cron/report-low-stock", handler.CronManualReportLowStock)
}
