package middleware

import (
	"net/http"
	"time"

	"github.com/boloc/go-frame-server/v2/pkg/errs"
	"github.com/boloc/go-frame-server/v2/pkg/frame/reqctx"
	"github.com/boloc/go-frame-server/v2/pkg/frame/webx"
	"github.com/boloc/go-frame-server/v2/pkg/logger"
	"github.com/gin-gonic/gin"
)

// AccessLog 在每个请求结束后打一条结构化访问日志（msg 固定 "http: access"），一个请求只有这一条。
// 请求开始时在 ctx 上开日志作用域：webx 的错误、GORM 的失败 SQL、业务用 logger.AddToScope 挂的字段
// 都在这一条里带出。级别规则见 accessLogLevel。
// 与 monitor.HTTPMetrics 都应 Use 在 ContextMiddleware 之后（要拿 request_id/client_ip）、业务路由之前。
func AccessLog(opts ...AccessLogOption) gin.HandlerFunc {
	cfg := applyAccessLogOptions(opts...)
	skip := make(map[string]struct{}, len(cfg.skipPaths))
	for _, p := range cfg.skipPaths {
		skip[p] = struct{}{}
	}

	return func(c *gin.Context) {
		if _, ok := skip[c.Request.URL.Path]; ok {
			c.Next()
			return
		}
		// 浏览器跨域会先打 OPTIONS 预检，成功是 204 且没有业务码。
		// 记访问日志只会刷屏；预检本身也不代表一次业务调用。
		if c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}

		ctx := logger.NewScope(c.Request.Context())
		c.Request = c.Request.WithContext(ctx)

		start := time.Now()
		c.Next()
		latency := time.Since(start)
		scopeFields := logger.EndScope(ctx)

		bizCode := errs.Code(-1)
		hasBiz := false
		if code, ok := webx.BizCode(c); ok {
			bizCode = code
			hasBiz = true
		}

		slow := cfg.slowThreshold > 0 && latency >= cfg.slowThreshold
		rc := reqctx.FromGin(c)
		clientIP := rc.ClientIP
		if clientIP == "" {
			clientIP = c.ClientIP()
		}

		// 顺序按控制台从左往右读：先结果，再错误详情（作用域字段），最后是排查用的上下文。
		fields := []logger.Field{
			logger.Int("status", c.Writer.Status()),
			logger.Int("biz_code", int(bizCode)),
			logger.Duration("latency", latency),
			logger.String("method", c.Request.Method),
			logger.String("route", c.FullPath()),
		}
		if slow {
			fields = append(fields, logger.Bool("slow", true))
		}
		if errStr := c.Errors.String(); errStr != "" {
			fields = append(fields, logger.String("errors", errStr))
		}
		fields = append(fields, scopeFields...)
		fields = append(fields,
			logger.String("client_ip", clientIP),
			logger.String("request_id", rc.RequestID),
			logger.String("user_agent", c.Request.UserAgent()),
			logger.Int("bytes_out", c.Writer.Size()),
			logger.String("path", c.Request.URL.Path),
		)

		logger.Log(accessLogLevel(c.Writer.Status(), bizCode, hasBiz, slow), "http: access", fields...)
	}
}

// AccessLogOption 配置 AccessLog。
type AccessLogOption func(*accessLogConfig)

type accessLogConfig struct {
	skipPaths     []string
	slowThreshold time.Duration
}

func defaultAccessLogSkipPaths() []string {
	return []string{"/health", "/livez", "/readyz", "/metrics"}
}

func applyAccessLogOptions(opts ...AccessLogOption) accessLogConfig {
	cfg := accessLogConfig{skipPaths: defaultAccessLogSkipPaths()}
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}

// WithSkipPaths 覆盖默认跳过路径（/health、/livez、/readyz、/metrics）。传入即覆盖，不会与默认合并。
func WithSkipPaths(paths ...string) AccessLogOption {
	return func(c *accessLogConfig) {
		c.skipPaths = paths
	}
}

// WithSlowThreshold 超过该耗时的请求加 slow=true，且级别至少 Warn。默认 0 不启用。
func WithSlowThreshold(d time.Duration) AccessLogOption {
	return func(c *accessLogConfig) {
		c.slowThreshold = d
	}
}

// 业务码 5xxxx 是 Error，其它业务失败是 Warn。没走 webx 的 HTTP 5xx（panic 恢复）只到 Warn：
// panic 的 ERROR 和堆栈由恢复处记，这里靠 request_id 串起来。
func accessLogLevel(status int, bizCode errs.Code, hasBiz, slow bool) logger.Level {
	if hasBiz && bizCode >= errs.CodeInternal {
		return logger.ErrorLevel
	}
	if hasBiz && bizCode != errs.CodeOK {
		return logger.WarnLevel
	}
	// biz_code=-1 是「没走 webx」，不是业务失败。OPTIONS 204 就属于这种。
	if status >= 400 || slow {
		return logger.WarnLevel
	}
	return logger.InfoLevel
}
