package route

import (
	"github.com/boloc/go-frame-server/internal/example/constant"
	"github.com/boloc/go-frame-server/internal/example/handler"
	"github.com/boloc/go-frame-server/pkg/frame"
	"github.com/boloc/go-frame-server/pkg/frame/components"
	"github.com/boloc/go-frame-server/pkg/frame/healthcheck"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// RegisterRoutes 注册所有路由。按域拆到同包的多个文件，这里只留入口和健康检查/指标/系统配置。
func RegisterRoutes(r *gin.Engine, metricsHandlers []gin.HandlerFunc) {
	// 健康检查：检查 MySQL/Redis 连通性。log_db 为非关键依赖，挂了不让整体变 503。
	r.GET("/health", healthcheck.Handler([]healthcheck.Dependency{
		{Name: constant.MySQLDefaultDB, Critical: true, Check: healthcheck.MySQLChecker(frame.TryDefaultDB)},
		{Name: constant.MySQLConfigDB, Critical: true, Check: healthcheck.MySQLChecker(func() (*gorm.DB, bool) {
			return components.TryMasterDB(constant.MySQLConfigDB)
		})},
		{Name: constant.MySQLLogDB, Critical: false, Check: healthcheck.MySQLChecker(func() (*gorm.DB, bool) {
			return components.TryMasterDB(constant.MySQLLogDB)
		})},
		{Name: "redis", Critical: true, Check: healthcheck.RedisChecker(frame.TryGetRedisCmdable)},
	}))

	// Prometheus 指标：进程级 + 连接池。生产环境建议配置 prometheus.password。
	if len(metricsHandlers) > 0 {
		r.GET("/metrics", metricsHandlers...)
	}

	// 系统配置接口：读写独立的 config_db。
	r.GET("/api/system-configs/:key", handler.SystemConfigGetByKey)
	r.POST("/api/system-configs/:key", handler.SystemConfigSet)

	// 注册业务路由
	registerProductRoutes(r)
	// 注册定时任务路由
	registerCronRoutes(r)
	// 注册订单路由
	registerOrderRoutes(r)
	// 注册能力路由
	registerCapabilityRoutes(r)
	// 注册出站 HTTP 客户端能力演示路由
	registerHTTPClientRoutes(r)
	// 注册对象存储能力演示路由
	registerStorageRoutes(r)
}
