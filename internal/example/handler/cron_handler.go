package handler

import (
	"github.com/boloc/go-frame-server/internal/example/repository"
	"github.com/boloc/go-frame-server/pkg/frame/webx"
	"github.com/gin-gonic/gin"
)

// CronManualReportLowStock 手动立即执行一次库存检查，直接调用任务背后的同一方法。
// POST /api/cron/report-low-stock
func CronManualReportLowStock(c *gin.Context) {
	if err := repository.DefaultOrderRepository().ReportLowStock(c.Request.Context()); err != nil {
		webx.Fail(c, err)
		return
	}
	webx.Success(c, gin.H{"message": "库存检查已执行"})
}
