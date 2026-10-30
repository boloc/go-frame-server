// Package ratelimit 基于 Redis 的限流。
//
// 三种算法：
//   - Smooth：平滑限流（GCRA / 令牌桶）。额度连续恢复，没有窗口边界可钻；用于全局兜底、按用户限速。
//   - Fixed：固定窗口。一个计数累加，一轮满了等这一轮结束；最省最直观，交界处可能短时打出 2×limit。
//   - Sliding：滑动窗口。任意连续 period 内严格最多 limit 次，逐条记时间，次数少时才用。
//
// 都可以 WithBan：一段时间内被拒到一定次数就封禁，封禁时长逐级加长。
//
// 用法有两种：
//   - Middleware：挂在 engine / 分组 / 单条路由上，挂在哪一层，这一层的请求共用一份额度
//     （默认按客户端 IP 区分）。
//   - Allow / Status / Reset：在 logic 里按账号、手机号这类要先解析请求体才拿得到的维度限流，
//     或者只给失败计数。
package ratelimit

import (
	"context"
	"errors"
	"math"
	"slices"
	"strconv"
	"time"

	"github.com/boloc/go-frame-server/v2/pkg/alert"
	"github.com/boloc/go-frame-server/v2/pkg/errs"
	"github.com/boloc/go-frame-server/v2/pkg/frame/rediskey"
	"github.com/boloc/go-frame-server/v2/pkg/frame/reqctx"
	"github.com/boloc/go-frame-server/v2/pkg/frame/webx"
	"github.com/boloc/go-frame-server/v2/pkg/logger"
	"github.com/gin-gonic/gin"
)

// KeyFunc 从请求里取限流对象。返回空字符串时这次请求不限流。
type KeyFunc func(c *gin.Context) string

// ByIP 按客户端 IP。挂在哪一层，这一层的所有接口共用一份额度。
func ByIP(c *gin.Context) string {
	if ip := reqctx.FromGin(c).ClientIP; ip != "" {
		return ip
	}
	return c.ClientIP()
}

// ByRoute 按路由 + 客户端 IP。同一层里每条路由各算各的。
func ByRoute(c *gin.Context) string {
	return c.FullPath() + "|" + ByIP(c)
}

// SkipPaths 按请求路径精确匹配跳过限流，通常用来放过 /health、/metrics 这类探针。
// 探针被限流会让编排系统误判实例不健康。
func SkipPaths(paths ...string) func(c *gin.Context) bool {
	return func(c *gin.Context) bool {
		return slices.Contains(paths, c.Request.URL.Path)
	}
}

// Options 中间件配置。
type Options struct {
	// KeyFunc 按什么区分限流对象，默认 ByIP。
	KeyFunc KeyFunc
	// Skip 返回 true 时这次请求不限流。
	Skip func(c *gin.Context) bool
	// FailOpen Redis 不可用时是否放行，默认 true。
	FailOpen bool
	// UseRealStatus 超限时是否返回 HTTP 429，默认 true。false 时 HTTP 200 + body.code=42900。
	UseRealStatus bool
	// OnLimited 被拒时回调，用来告警或记录。在写响应之前调用。
	OnLimited func(c *gin.Context, key string, res Result)
	// Limiter 执行限流的 Redis，默认是框架的 Redis。
	Limiter *Limiter
}

// Option 中间件的可选配置，传给 Middleware。
type Option func(*Options)

// WithKeyFunc 按什么区分限流对象，默认 ByIP。
// 同一层里每条路由各算各的用 ByRoute；登录后按用户限速传自己的函数，返回用户 ID。
func WithKeyFunc(fn KeyFunc) Option { return func(o *Options) { o.KeyFunc = fn } }

// WithSkip 返回 true 的请求不限流，也不计数。通常配 SkipPaths 放过 /health、/metrics。
func WithSkip(fn func(c *gin.Context) bool) Option { return func(o *Options) { o.Skip = fn } }

// WithFailOpen Redis 不可用时怎么办，默认 true。
// true 放行并告警，这段时间没有限流；false 直接拒绝，返回 50002。
// 限流用来保护扛不住的下游时用 false。
func WithFailOpen(failOpen bool) Option { return func(o *Options) { o.FailOpen = failOpen } }

// WithUseRealStatus 超限时的 HTTP 状态码，默认 true。
// true 返回 HTTP 429，网关、负载均衡和客户端的退避逻辑认得；
// false 返回 HTTP 200 + body.code=42900，给只看 body.code 的前端用。
func WithUseRealStatus(useRealStatus bool) Option {
	return func(o *Options) { o.UseRealStatus = useRealStatus }
}

// WithOnLimited 被拒时调用 fn，用来告警或记录。key 是限流对象（默认是 IP），
// res.Banned 表示正处于封禁期。fn 在写 429 响应之前同步执行，不要在里面做慢操作。
func WithOnLimited(fn func(c *gin.Context, key string, res Result)) Option {
	return func(o *Options) { o.OnLimited = fn }
}

// WithLimiter 换一个 Limiter，默认用框架的 Redis。测试或接另一个 Redis 时用。
func WithLimiter(l *Limiter) Option { return func(o *Options) { o.Limiter = l } }

// Middleware 按 rule 限流。默认按客户端 IP，Redis 不可用时放行，超限返回 HTTP 429。
//
// rule 决定限多少，三选一，可再接 .WithBan：
//
//	ratelimit.Smooth(name, limit, period, burst)  平滑限流，额度连续恢复
//	ratelimit.Fixed(name, limit, period)          固定窗口，一个计数累加
//	ratelimit.Sliding(name, limit, period)        滑动窗口，严格计数
//	.WithBan(ratelimit.Ban{...})                  一直被拒就封禁，时长逐级加长
//
// opts 决定按谁算、出错怎么办，全部可选：
//
//	ratelimit.WithKeyFunc(fn)          按谁计数，默认 ratelimit.ByIP；ratelimit.ByRoute 是接口 + IP
//	ratelimit.WithSkip(fn)             哪些请求不限流，例如 ratelimit.SkipPaths("/health")
//	ratelimit.WithFailOpen(bool)       Redis 不可用时放行（默认 true）还是拒绝
//	ratelimit.WithUseRealStatus(bool)  超限回 HTTP 429（默认 true）还是 200 + code 42900
//	ratelimit.WithOnLimited(fn)        被拒时回调，用来告警
//	ratelimit.WithLimiter(l)           换一个 Redis，测试用
//
// 规则名决定计数：全局一层和分组一层各用各的规则名，就是两份独立的额度，请求要两层都过。
// 响应头：X-RateLimit-Limit 是规则容量，X-RateLimit-Remaining 是还能立刻放行几次，
// X-RateLimit-Reset 是额度完全恢复还要几秒；被拒时 Retry-After 是至少还要等几秒。
func Middleware(rule Rule, opts ...Option) gin.HandlerFunc {
	rule.mustValid()
	// 路由注册阶段就确认命名空间已注入，漏了直接启动失败，而不是等第一个请求 panic。
	_ = rediskey.Prefix(rediskey.SegRateLimit)

	o := &Options{
		KeyFunc:       ByIP,
		FailOpen:      true,
		UseRealStatus: true,
		Limiter:       defaultLimiter,
	}
	for _, opt := range opts {
		opt(o)
	}
	if o.KeyFunc == nil {
		panic("ratelimit: KeyFunc 不能为 nil")
	}
	if o.Limiter == nil {
		panic("ratelimit: Limiter 不能为 nil")
	}

	return func(c *gin.Context) {
		if o.Skip != nil && o.Skip(c) {
			c.Next()
			return
		}
		key := o.KeyFunc(c)
		if key == "" {
			c.Next()
			return
		}

		res, err := o.Limiter.Allow(c.Request.Context(), rule, key)
		if err != nil {
			// 请求 context 已取消，Redis 没有故障。不发降级告警，额度也不扣。
			if errors.Is(err, context.Canceled) {
				c.Next()
				return
			}
			degrade(c, o, rule, err)
			return
		}

		c.Header("X-RateLimit-Limit", strconv.Itoa(res.Limit))
		c.Header("X-RateLimit-Remaining", strconv.Itoa(res.Remaining))
		c.Header("X-RateLimit-Reset", strconv.Itoa(ceilSeconds(res.ResetAfter)))

		if !res.Allowed {
			if o.OnLimited != nil {
				o.OnLimited(c, key, res)
			}
			reject(c, o, res)
			return
		}
		c.Next()
	}
}

func degrade(c *gin.Context, o *Options, rule Rule, err error) {
	fields := []logger.Field{logger.String("rule", rule.name), logger.String("route", c.FullPath()), logger.Err(err)}

	alert.Notify(c.Request.Context(), alert.Event{
		Scope: "ratelimit", Name: rule.name, Message: "degraded", Err: err,
		Fields: map[string]any{"route": c.FullPath(), "fail_open": o.FailOpen},
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

func reject(c *gin.Context, o *Options, res Result) {
	c.Header("Retry-After", strconv.Itoa(max(ceilSeconds(res.RetryAfter), 1)))
	e := errs.New(errs.CodeTooManyRequests, "")
	if o.UseRealStatus {
		webx.FailWithStatus(c, e)
	} else {
		webx.Fail(c, e)
	}
	c.Abort()
}

func ceilSeconds(d time.Duration) int {
	if d <= 0 {
		return 0
	}
	return int(math.Ceil(d.Seconds()))
}
