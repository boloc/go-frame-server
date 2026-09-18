package bootstrap

import (
	"strconv"
	"time"

	"github.com/boloc/go-frame-server/cmd/example/route"
	"github.com/boloc/go-frame-server/pkg/frame"
	"github.com/boloc/go-frame-server/pkg/frame/components"
	"github.com/boloc/go-frame-server/pkg/frame/config"
	"github.com/boloc/go-frame-server/pkg/frame/middleware"
	"github.com/boloc/go-frame-server/pkg/monitor"

	"github.com/gin-gonic/gin"
)

// ServerConfig 对应配置段 server。
type ServerConfig struct {
	Env string `mapstructure:"env" validate:"required,oneof=local dev test production"`
	// Name 是进程标识，同时也是 Redis key 的命名空间。
	Name           string        `mapstructure:"name" validate:"required"`
	Port           int           `mapstructure:"port" validate:"required,min=1,max=65535"`
	Timezone       string        `mapstructure:"timezone"`
	TrustedProxies []string      `mapstructure:"trusted_proxies"`
	RequestTimeout time.Duration `mapstructure:"request_timeout"`
}

// SetupGin 初始化 Gin 组件。metricsHandlers 是 /metrics 端点的 handler 链，不需要时传 nil。
func SetupGin(f *frame.Frame, conf *config.ConfigComponent, metricsHandlers []gin.HandlerFunc) {
	var cfg ServerConfig
	conf.MustStrictUnmarshalKey("server", &cfg)

	opts := []components.GinOption{
		components.WithGinPort(strconv.Itoa(cfg.Port)),
		components.WithGinMode(components.GinModeForEnv(cfg.Env)),
		components.WithGinShutdownTimeout(5 * time.Second),
		components.WithGinRouter(func(r *gin.Engine) {
			route.RegisterRoutes(r, metricsHandlers)
		}),
	}

	// TrustedProxies 契约（与 components.GinComponent.Start 对齐）：
	// - conf.IsSet("server.trusted_proxies") == true：必须调用 WithGinTrustedProxies。
	//   viper 把 `trusted_proxies: []` 解成 nil 时转成非 nil 空切片，表示"显式声明直连"。
	// - IsSet == false：不调用 WithGinTrustedProxies，保持 nil。
	//   Release 模式下 Start 会因此返回 error。
	if conf.IsSet("server.trusted_proxies") {
		proxies := cfg.TrustedProxies
		if proxies == nil {
			proxies = []string{}
		}
		opts = append(opts, components.WithGinTrustedProxies(proxies))
	}

	if cfg.RequestTimeout > 0 {
		opts = append(opts, components.WithGinRequestTimeout(cfg.RequestTimeout))
	}

	ginComponent := components.NewGinComponent(opts...)

	// 全局中间件，顺序有意义：
	//   ContextMiddleware —— 生成/回显 X-Request-Id，解析 ClientIP，缓存请求体；
	//   AccessLog        —— 每个请求一条结构化访问日志（msg=http: access，含 biz_code/
	//                       request_id/耗时）。默认跳过 /health /livez /readyz /metrics；
	//                       WithSlowThreshold(1s) 超过 1s 打 Warn 并加 slow=true。
	//   HTTPMetrics      —— http_requests_total{method,route,status,biz_code} /
	//                       http_request_duration_seconds / http_requests_in_flight。
	//                       biz_code 来自 webx.BizCode（Success/Fail 写入）。
	// 后两个都要读 ContextMiddleware 写进去的字段，所以必须排在它之后、业务路由之前。
	// MaxBodyBytes（默认 4MB，演示见 POST /test/body-limit）和 RequestTimeout
	// （server.request_timeout，演示见 GET /test/slow）由 NewGinComponent 在这之前挂好。
	ginComponent.Use(
		middleware.ContextMiddleware(),
		middleware.AccessLog(middleware.WithSlowThreshold(time.Second)),
		monitor.HTTPMetrics(),
	)
	f.RegisterComponent(ginComponent)
}
