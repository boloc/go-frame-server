package route

import (
	examplemw "github.com/boloc/go-frame-server/v2/internal/example/middleware"

	"github.com/boloc/go-frame-server/v2/internal/example/handler"
	"github.com/gin-gonic/gin"
)

// registerClickHouseRoutes 演示 ClickHouse GORM 读写。未启用时接口返回业务错误，不 panic。
//
//	curl -H "X-Demo-Token: x" -H "Content-Type: application/json" \
//	  -d '{"name":"demo"}' localhost:10006/test/clickhouse/events
func registerClickHouseRoutes(r *gin.Engine) {
	group := r.Group("/test/clickhouse")
	group.Use(examplemw.RequireDemoToken())
	{
		group.POST("/events", handler.ClickHouseEventRoundTrip)
	}
}
