package bootstrap

import (
	"github.com/boloc/go-frame-server/pkg/frame"
	"github.com/boloc/go-frame-server/pkg/frame/components"
	"github.com/boloc/go-frame-server/pkg/frame/config"
	"github.com/boloc/go-frame-server/pkg/monitor"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// SetupMonitor 注册进程级指标和 MySQL/Redis 连接池采集器，返回 /metrics 的 handler 链。
// redisComponent 可以是单机、集群或哨兵组件；配置了 prometheus.password 时，链上会带 Basic Auth。
func SetupMonitor(
	f *frame.Frame,
	conf *config.ConfigComponent,
	mysqlInstances map[string]*components.MySQLComponent,
	redisComponent components.RedisPoolStatsProvider,
) []gin.HandlerFunc {
	metricsComponent := monitor.NewMetricsComponent()
	f.RegisterComponent(metricsComponent)

	if len(mysqlInstances) > 0 {
		prometheus.MustRegister(components.NewMySQLPoolCollector(mysqlInstances))
	}
	if redisComponent != nil {
		prometheus.MustRegister(components.NewRedisPoolCollector("default", redisComponent))
	}

	metricsHandler := gin.WrapH(promhttp.Handler())

	password := conf.GetString("prometheus.password")
	if password == "" {
		return []gin.HandlerFunc{metricsHandler}
	}
	return []gin.HandlerFunc{monitor.PrometheusAuth(password), metricsHandler}
}
