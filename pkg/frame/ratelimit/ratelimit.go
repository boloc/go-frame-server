// Package ratelimit 提供基于 Redis 固定窗口的限流中间件。按路由显式挂载。
package ratelimit

import (
	"strconv"
	"time"

	"github.com/boloc/go-frame-server/pkg/alert"
	"github.com/boloc/go-frame-server/pkg/errs"
	"github.com/boloc/go-frame-server/pkg/frame"
	"github.com/boloc/go-frame-server/pkg/frame/reqctx"
	"github.com/boloc/go-frame-server/pkg/frame/webx"
	"github.com/boloc/go-frame-server/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const DefaultKeyPrefix = "ratelimit:"

const fixedWindowScript = `
local current = redis.call("INCR", KEYS[1])
if current == 1 then
	redis.call("EXPIRE", KEYS[1], ARGV[1])
end
return current
`

// Options 限流配置。Limit/Window 必须显式设置。
type Options struct {
	Limit     int64
	Window    time.Duration
	KeyPrefix string
	KeyFunc   func(c *gin.Context) string
	// FailOpen Redis 不可用时是否放行，默认 true。
	FailOpen bool
	// UseRealStatus 超限时是否返回 HTTP 429 + Retry-After，默认 true。
	UseRealStatus bool
	Redis         func() (redis.Cmdable, bool)
}

type Option func(*Options)

func WithLimit(limit int64) Option { return func(o *Options) { o.Limit = limit } }

func WithWindow(window time.Duration) Option { return func(o *Options) { o.Window = window } }

func WithKeyPrefix(prefix string) Option { return func(o *Options) { o.KeyPrefix = prefix } }

func WithKeyFunc(fn func(c *gin.Context) string) Option {
	return func(o *Options) { o.KeyFunc = fn }
}

func WithFailOpen(failOpen bool) Option { return func(o *Options) { o.FailOpen = failOpen } }

func WithUseRealStatus(useRealStatus bool) Option {
	return func(o *Options) { o.UseRealStatus = useRealStatus }
}

func WithRedis(fn func() (redis.Cmdable, bool)) Option { return func(o *Options) { o.Redis = fn } }

func defaultKeyFunc(c *gin.Context) string {
	ip := reqctx.FromGin(c).ClientIP
	if ip == "" {
		ip = c.ClientIP()
	}
	return c.FullPath() + ":" + ip
}

func defaultOptions() *Options {
	return &Options{
		KeyPrefix:     DefaultKeyPrefix,
		KeyFunc:       defaultKeyFunc,
		FailOpen:      true,
		UseRealStatus: true,
		Redis:         frame.TryGetRedisCmdable,
	}
}

// Middleware 创建限流中间件。Limit/Window 未设置时 panic。
func Middleware(opts ...Option) gin.HandlerFunc {
	o := defaultOptions()
	for _, opt := range opts {
		opt(o)
	}
	if o.Limit <= 0 || o.Window <= 0 {
		panic("ratelimit: Limit 和 Window 必须显式设置为正数，用 WithLimit/WithWindow")
	}

	windowSeconds := int64(o.Window / time.Second)
	if windowSeconds <= 0 {
		windowSeconds = 1
	}

	return func(c *gin.Context) {
		client, ok := o.Redis()
		if !ok {
			degrade(c, o, "redis unavailable", nil)
			return
		}

		key := o.KeyPrefix + o.KeyFunc(c)
		count, err := client.Eval(c.Request.Context(), fixedWindowScript, []string{key}, windowSeconds).Int64()
		if err != nil {
			degrade(c, o, "EVAL failed", err)
			return
		}

		remaining := o.Limit - count
		if remaining < 0 {
			remaining = 0
		}
		c.Header("X-RateLimit-Limit", strconv.FormatInt(o.Limit, 10))
		c.Header("X-RateLimit-Remaining", strconv.FormatInt(remaining, 10))

		if count > o.Limit {
			reject(c, o, windowSeconds)
			return
		}
		c.Next()
	}
}

func degrade(c *gin.Context, o *Options, reason string, err error) {
	fields := []zap.Field{zap.String("reason", reason), zap.String("route", c.FullPath())}
	if err != nil {
		fields = append(fields, zap.Error(err))
	}

	alert.Notify(c.Request.Context(), alert.Event{
		Scope: "ratelimit", Name: c.FullPath(), Message: "degraded: " + reason, Err: err,
		Fields: map[string]any{"fail_open": o.FailOpen},
	})

	if o.FailOpen {
		logger.Warn("ratelimit: degraded, falling back to no protection (FailOpen=true)", fields...)
		c.Next()
		return
	}

	logger.Warn("ratelimit: degraded, rejecting request (FailOpen=false)", fields...)
	webx.Fail(c, errs.New(errs.CodeCache, "限流服务暂时不可用，请稍后重试"))
	c.Abort()
}

func reject(c *gin.Context, o *Options, windowSeconds int64) {
	c.Header("Retry-After", strconv.FormatInt(windowSeconds, 10))
	e := errs.New(errs.CodeTooManyRequests, "")
	if o.UseRealStatus {
		webx.FailWithStatus(c, e)
	} else {
		webx.Fail(c, e)
	}
	c.Abort()
}
