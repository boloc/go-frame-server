package monitor

import (
	"fmt"
	"go-frame-server/pkg/frame/config"
	"runtime"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// 内存使用量
	memoryUsage = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "resource_memory_usage_bytes",
			Help: "资源内存使用量（字节）",
		},
	)

	// Goroutine数量
	goroutineCount = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "resource_goroutine_count",
			Help: "资源Goroutine数量",
		},
	)

	// 服务响应时间
	resourceLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "source_response_time_seconds",
			Help: "服务响应时间（秒）",
			// 根据你的服务响应时间分布，调整这些值
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
		},
		[]string{"endpoint"},
	)

	// 错误监控
	sourceError = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "source_error_count",
			Help: "服务错误监控",
		},
		[]string{"endpoint", "error_code"},
	)
)

func init() {
	// 注册所有指标
	// 注意：使用promauto已自动注册，不需要再显式注册
	fmt.Println("Prometheus metrics initializing...")
	// 开始定期收集资源使用情况
	go collectResourceMetrics()
}

// collectResourceMetrics 定期收集资源使用情况
func collectResourceMetrics() {
	// 立即更新一次数据
	updateMetrics()

	// 开始定期更新
	for {
		fmt.Println("查看是否执行")
		time.Sleep(5 * time.Second)
		updateMetrics()
	}
}

// 将更新逻辑抽取出来
func updateMetrics() {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// 更新内存使用量（字节）
	memoryUsage.Set(float64(memStats.Alloc))

	// 更新Goroutine数量
	goroutineCount.Set(float64(runtime.NumGoroutine()))
}

// ObserveLatency 记录响应时间
func ObserveLatency(endpoint string, duration time.Duration) {
	resourceLatency.WithLabelValues(endpoint).Observe(duration.Seconds())
}

// ObserveHTTPError 记录错误
func ObserveError(endpoint string, errorCode int) {
	sourceError.WithLabelValues(endpoint, strconv.Itoa(errorCode)).Inc()
}

// 添加基本认证
func PrometheusAuth() gin.HandlerFunc {
	return gin.BasicAuth(gin.Accounts{
		"prometheus": config.GetConfig().GetString("prometheus.password"),
	})
}
