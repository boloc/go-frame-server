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
// 请求参数记成 query、body（JSON/表单），各自截断到 WithParamMaxBytes（默认 4KB），
// 截断时值末尾带 "…[truncated, N bytes total]" 并加 query_truncated / body_truncated；
// 哪些字段的值要记成 ***，由业务用 WithSensitiveKeys 指定，框架不内置。
// 与 monitor.HTTPMetrics 都应 Use 在 ContextMiddleware 之后（要拿 request_id/client_ip）、业务路由之前。
func AccessLog(opts ...AccessLogOption) gin.HandlerFunc {
	cfg := applyAccessLogOptions(opts...)
	masker := newParamMasker(cfg.sensitiveKeys)
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
		bc, kind := captureBody(c, cfg.paramMaxBytes)

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
		fields = append(fields, paramFields(c, cfg.paramMaxBytes, masker, bc, kind)...)
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
	paramMaxBytes int
	sensitiveKeys []string
}

func defaultAccessLogSkipPaths() []string {
	return []string{"/health", "/livez", "/readyz", "/metrics"}
}

func applyAccessLogOptions(opts ...AccessLogOption) accessLogConfig {
	cfg := accessLogConfig{
		skipPaths:     defaultAccessLogSkipPaths(),
		paramMaxBytes: defaultParamMaxBytes,
	}
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

// WithParamMaxBytes 设置 query、body 各自最多记录的字节数，超出截断并加 query_truncated / body_truncated。
// 默认 4096；传 0 或负数不记录 query 和 body。
func WithParamMaxBytes(n int) AccessLogOption {
	return func(c *accessLogConfig) {
		c.paramMaxBytes = n
	}
}

// WithSensitiveKeys 指定 query、body 里要脱敏的字段名（不区分大小写，按子串匹配），值记成 ***。
// 多次调用会合并。不调用时不脱敏。
func WithSensitiveKeys(keys ...string) AccessLogOption {
	return func(c *accessLogConfig) {
		c.sensitiveKeys = append(c.sensitiveKeys, keys...)
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
