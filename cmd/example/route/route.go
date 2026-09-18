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
	// /readyz 接 k8s readinessProbe：依赖挂了摘流量。/health 保留兼容别名，复用同一 handler。
	// /livez 接 k8s livenessProbe：只表示进程活着，不检查依赖。
	// 不要把 readyz/Handler 接到 liveness 上，否则 Redis 一挂所有 Pod 会被循环重启。
	readyz := healthcheck.Handler([]healthcheck.Dependency{
		{Name: constant.MySQLDefaultDB, Critical: true, Check: healthcheck.MySQLChecker(frame.TryDefaultDB)},
		{Name: constant.MySQLConfigDB, Critical: true, Check: healthcheck.MySQLChecker(func() (*gorm.DB, bool) {
			return components.TryMasterDB(constant.MySQLConfigDB)
		})},
		{Name: constant.MySQLLogDB, Critical: false, Check: healthcheck.MySQLChecker(func() (*gorm.DB, bool) {
			return components.TryMasterDB(constant.MySQLLogDB)
		})},
		{Name: "redis", Critical: true, Check: healthcheck.RedisChecker(frame.TryGetRedisCmdable)},
	})
	r.GET("/health", readyz)
	r.GET("/readyz", readyz)
	r.GET("/livez", healthcheck.Liveness())

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
	// 注册 ClickHouse GORM 读写演示路由
	registerClickHouseRoutes(r)

	registeredRouteCount = len(r.Routes())
}

// registeredRouteCount 在 RegisterRoutes 结束时写入，供 AfterStart 打印已注册路由数。
var registeredRouteCount int

// RegisteredRouteCount 返回最近一次 RegisterRoutes 登记的路由条数。
func RegisteredRouteCount() int {
	return registeredRouteCount
}
