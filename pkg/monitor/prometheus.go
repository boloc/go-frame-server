// Package monitor 提供进程级资源指标采集，通过 MetricsComponent 接入 Frame 生命周期。
package monitor

import (
	"context"
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

// MetricsConfig MetricsComponent 的配置。
type MetricsConfig struct {
	CollectInterval time.Duration
}

// MetricsOption 定义 MetricsComponent 选项函数类型。
type MetricsOption func(*MetricsConfig)

// WithCollectInterval 设置采集间隔。
func WithCollectInterval(d time.Duration) MetricsOption {
	return func(c *MetricsConfig) { c.CollectInterval = d }
}

// MetricsComponent 定期采集进程级资源指标，实现 frame.Component。
type MetricsComponent struct {
	config *MetricsConfig
	cancel context.CancelFunc
	done   chan struct{}
}

// NewMetricsComponent 创建资源指标采集组件。
func NewMetricsComponent(opts ...MetricsOption) *MetricsComponent {
	cfg := &MetricsConfig{CollectInterval: 15 * time.Second}
	for _, opt := range opts {
		opt(cfg)
	}
	return &MetricsComponent{config: cfg}
}

func (m *MetricsComponent) Start(ctx context.Context) error {
	updateMetrics()

	runCtx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.done = make(chan struct{})
	go m.loop(runCtx)
	return nil
}

func (m *MetricsComponent) loop(ctx context.Context) {
	defer close(m.done)

	ticker := time.NewTicker(m.config.CollectInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			updateMetrics()
		}
	}
}

func (m *MetricsComponent) Stop(ctx context.Context) error {
	if m.cancel == nil {
		return nil
	}
	m.cancel()

	select {
	case <-m.done:
	case <-ctx.Done():
	}
	return nil
}

// updateMetrics 采集一次内存/goroutine 指标。
func updateMetrics() {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	memoryUsage.Set(float64(memStats.Alloc))
	goroutineCount.Set(float64(runtime.NumGoroutine()))
}

// ObserveLatency 记录响应时间
func ObserveLatency(endpoint string, duration time.Duration) {
	resourceLatency.WithLabelValues(endpoint).Observe(duration.Seconds())
}

// ObserveError 记录错误
func ObserveError(endpoint string, errorCode int) {
	sourceError.WithLabelValues(endpoint, strconv.Itoa(errorCode)).Inc()
}

// PrometheusAuth 给 /metrics 加 HTTP Basic Auth，password 由调用方传入。
func PrometheusAuth(password string) gin.HandlerFunc {
	return gin.BasicAuth(gin.Accounts{
		"prometheus": password,
	})
}
