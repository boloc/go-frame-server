package middleware

import (
	"time"

	"github.com/boloc/go-frame-server/v2/pkg/errs"
	"github.com/boloc/go-frame-server/v2/pkg/frame/reqctx"
	"github.com/boloc/go-frame-server/v2/pkg/frame/webx"
	"github.com/boloc/go-frame-server/v2/pkg/logger"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// AccessLog 在每个请求结束后打一条结构化访问日志（msg 固定 "http: access"）。
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

		start := time.Now()
		c.Next()
		latency := time.Since(start)

		bizCode := errs.Code(-1)
		if code, ok := webx.BizCode(c); ok {
			bizCode = code
		}

		slow := cfg.slowThreshold > 0 && latency >= cfg.slowThreshold
		rc := reqctx.FromGin(c)
		clientIP := rc.ClientIP
		if clientIP == "" {
			clientIP = c.ClientIP()
		}

		fields := []zap.Field{
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.String("route", c.FullPath()),
			zap.Int("status", c.Writer.Status()),
			zap.Int("biz_code", int(bizCode)),
			zap.Duration("latency", latency),
			zap.String("client_ip", clientIP),
			zap.String("request_id", rc.RequestID),
			zap.String("user_agent", c.Request.UserAgent()),
			zap.Int("bytes_out", c.Writer.Size()),
		}
		if errStr := c.Errors.String(); errStr != "" {
			fields = append(fields, zap.String("errors", errStr))
		}
		if slow {
			fields = append(fields, zap.Bool("slow", true))
		}

		switch accessLogLevel(c.Writer.Status(), bizCode, slow) {
		case zapcore.ErrorLevel:
			logger.Error("http: access", fields...)
		case zapcore.WarnLevel:
			logger.Warn("http: access", fields...)
		default:
			logger.Info("http: access", fields...)
		}
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

func accessLogLevel(status int, bizCode errs.Code, slow bool) zapcore.Level {
	if status >= 500 || bizCode >= errs.CodeInternal {
		return zapcore.ErrorLevel
	}
	if status >= 400 || bizCode != 0 || slow {
		return zapcore.WarnLevel
	}
	return zapcore.InfoLevel
}
