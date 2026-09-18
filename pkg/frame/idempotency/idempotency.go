// Package idempotency 提供基于 Redis 的幂等中间件。幂等键由客户端传入，与 RequestID 无关。
package idempotency

import (
	"bytes"
	"context"
	"encoding/json"
	"time"

	"github.com/boloc/go-frame-server/v2/pkg/alert"
	"github.com/boloc/go-frame-server/v2/pkg/errs"
	"github.com/boloc/go-frame-server/v2/pkg/frame"
	"github.com/boloc/go-frame-server/v2/pkg/frame/rediskey"
	"github.com/boloc/go-frame-server/v2/pkg/frame/webx"
	"github.com/boloc/go-frame-server/v2/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const DefaultHeader = "Idempotency-Key"
const DefaultTTL = 24 * time.Hour

const processingMarker = "\x00processing"

type redisClient interface {
	SetNX(ctx context.Context, key string, value any, expiration time.Duration) *redis.BoolCmd
	Get(ctx context.Context, key string) *redis.StringCmd
	Set(ctx context.Context, key string, value any, expiration time.Duration) *redis.StatusCmd
	Del(ctx context.Context, keys ...string) *redis.IntCmd
}

type Options struct {
	Header    string
	KeyPrefix string
	TTL       time.Duration
	// Required 为 true 时，缺少幂等键直接拒绝。
	Required bool
	// FailOpen 控制 Redis 不可用（拿不到 client 或 SETNX 失败）时的降级：
	// true（默认）放行并失去幂等保护；false 拒绝请求。金融/支付类应显式设为 false。
	// 只影响 Redis 故障，不影响"键冲突 / 重放"等正常路径。
	FailOpen bool
	// ScopeFunc 把调用方维度拼进 Redis key。返回非空时 key 为
	// KeyPrefix + FullPath + ":" + scope + ":" + 幂等键；返回空则保持
	// KeyPrefix + FullPath + ":" + 幂等键。
	//
	// 需要：键由客户端生成且可能跨用户重复，或要防止猜别人的键重放响应。
	// 不需要：键是全局唯一 UUID，或接口没有用户概念。
	ScopeFunc func(c *gin.Context) string
	Redis     func() (redis.Cmdable, bool)
}

// Option 函数式选项。
type Option func(*Options)

func WithHeader(header string) Option    { return func(o *Options) { o.Header = header } }
func WithKeyPrefix(prefix string) Option { return func(o *Options) { o.KeyPrefix = prefix } }
func WithTTL(ttl time.Duration) Option   { return func(o *Options) { o.TTL = ttl } }
func WithRequired(required bool) Option  { return func(o *Options) { o.Required = required } }

func WithFailOpen(failOpen bool) Option { return func(o *Options) { o.FailOpen = failOpen } }

// WithScopeFunc 设置幂等键作用域。fn 返回非空时拼进 Redis key，返回空时 key 格式不变。
func WithScopeFunc(fn func(c *gin.Context) string) Option {
	return func(o *Options) { o.ScopeFunc = fn }
}

func WithRedis(fn func() (redis.Cmdable, bool)) Option { return func(o *Options) { o.Redis = fn } }

// defaultOptions 在 Middleware 里调用，也就是路由注册阶段（GinComponent.Start 内），
// 此时配置已经加载、rediskey 命名空间已经注入，所以可以直接取前缀。
func defaultOptions() *Options {
	return &Options{
		Header:    DefaultHeader,
		KeyPrefix: rediskey.Prefix(rediskey.SegIdempotency),
		TTL:       DefaultTTL,
		Required:  false,
		FailOpen:  true,
		Redis:     frame.TryGetRedisCmdable,
	}
}

type cachedResponse struct {
	Status      int    `json:"status"`
	ContentType string `json:"content_type"`
	Body        string `json:"body"`
}

// codeEnvelope 的 Code 字段用指针，区分"字段值是 0（CodeOK）"和"这个字段压根不存在"
// （后者说明响应体不是 webx 格式，不该被当成成功缓存）。
type codeEnvelope struct {
	Code *errs.Code `json:"code"`
}

// Middleware 创建幂等中间件。缺少幂等键时按 Required 决定拒绝或放行；
// 已有最终结果则重放，处理中返回冲突；内部错误不缓存，允许重试。
//
// Redis key 默认是 KeyPrefix + FullPath + ":" + 请求头里的幂等键。
// 用 WithScopeFunc 可以把调用方维度（如用户 ID）拼进 key，避免不同用户传相同
// 幂等键时互相重放对方的响应。ScopeFunc 返回空字符串时 key 格式不变。
//
// 什么时候需要 ScopeFunc：键由客户端生成且可能跨用户重复（例如都传 "1"），
// 或需要防止恶意用户猜别人的键重放响应。
// 什么时候不需要：键是全局唯一 UUID，或接口没有用户概念。
//
// Redis 不可用（拿不到 client 或 SETNX 失败）时按 FailOpen 降级，行为不变：
// FailOpen=true（默认）不做保护、放行请求并告警；FailOpen=false 拒绝请求。
func Middleware(opts ...Option) gin.HandlerFunc {
	o := defaultOptions()
	for _, opt := range opts {
		opt(o)
	}

	return func(c *gin.Context) {
		key := c.GetHeader(o.Header)
		if key == "" {
			if o.Required {
				webx.Fail(c, errs.InvalidParams("缺少 "+o.Header+" 请求头，该接口要求幂等保护"))
				c.Abort()
				return
			}
			c.Next()
			return
		}

		client, ok := o.Redis()
		if !ok {
			degrade(c, o, "redis unavailable", key, nil)
			return
		}

		redisKey := buildRedisKey(o, c, key)
		ctx := c.Request.Context()

		claimed, err := client.SetNX(ctx, redisKey, processingMarker, o.TTL).Result()
		if err != nil {
			degrade(c, o, "SETNX failed", key, err)
			return
		}

		if !claimed {
			replayOrReject(c, client, redisKey)
			return
		}

		writer := &bodyCaptureWriter{ResponseWriter: c.Writer, buf: &bytes.Buffer{}}
		c.Writer = writer

		// 用 defer：handler panic 时 c.Next() 之后的语句不会执行，不这样写的话
		// SETNX 占的锁会一直卡到 TTL 过期才释放。finalize 用独立的 context（脱离
		// 请求 ctx 的取消信号），避免请求超时连带导致释放锁/落缓存也失败。
		defer func() {
			finalizeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
			defer cancel()
			finalize(finalizeCtx, client, redisKey, o.TTL, writer)
		}()

		c.Next()
	}
}

func buildRedisKey(o *Options, c *gin.Context, key string) string {
	base := o.KeyPrefix + c.FullPath()
	if o.ScopeFunc != nil {
		if scope := o.ScopeFunc(c); scope != "" {
			return base + ":" + scope + ":" + key
		}
	}
	return base + ":" + key
}

func degrade(c *gin.Context, o *Options, reason, key string, err error) {
	fields := []zap.Field{zap.String("reason", reason), zap.String("key", key), zap.String("route", c.FullPath())}
	if err != nil {
		fields = append(fields, zap.Error(err))
	}

	alert.Notify(c.Request.Context(), alert.Event{
		Scope: "idempotency", Name: key, Message: "degraded: " + reason, Err: err,
		Fields: map[string]any{"route": c.FullPath(), "fail_open": o.FailOpen},
	})

	if o.FailOpen {
		logger.Warn("idempotency: degraded, falling back to no protection (FailOpen=true)", fields...)
		c.Next()
		return
	}

	logger.Warn("idempotency: degraded, rejecting request (FailOpen=false)", fields...)
	webx.Fail(c, errs.New(errs.CodeCache, "幂等保护服务暂时不可用，请稍后重试"))
	c.Abort()
}

func replayOrReject(c *gin.Context, client redisClient, redisKey string) {
	defer c.Abort()

	cached, err := client.Get(c.Request.Context(), redisKey).Result()
	if err != nil || cached == processingMarker {
		webx.Fail(c, errs.Conflict("相同的幂等键正在处理中或刚被使用过，请稍后重试，不要重复提交"))
		return
	}

	var resp cachedResponse
	if err := json.Unmarshal([]byte(cached), &resp); err != nil {
		webx.Fail(c, errs.Conflict("重复请求"))
		return
	}
	// 重放的是缓存的原始字节，没有经过 webx.Success/Fail，所以 webx.BizCodeKey 不会被写入，
	// AccessLog/HTTPMetrics 会把这次请求记成 biz_code=-1/none 并当成异常（Warn）。这里把缓存
	// 体里的业务码解出来补上，让重放请求在日志和指标里跟首次请求长得一样。
	var env codeEnvelope
	if err := json.Unmarshal([]byte(resp.Body), &env); err == nil && env.Code != nil {
		c.Set(webx.BizCodeKey, *env.Code)
	}
	c.Data(resp.Status, resp.ContentType, []byte(resp.Body))
}

func finalize(ctx context.Context, client redisClient, redisKey string, ttl time.Duration, w *bodyCaptureWriter) {
	status := w.ResponseWriter.Status()
	body := w.buf.Bytes()

	if !shouldCache(status, body) {
		if err := client.Del(ctx, redisKey).Err(); err != nil {
			logger.Warn("idempotency: release lock failed", zap.Error(err), zap.String("key", redisKey))
		}
		return
	}

	payload, err := json.Marshal(cachedResponse{
		Status:      status,
		ContentType: w.Header().Get("Content-Type"),
		Body:        string(body),
	})
	if err != nil {
		logger.Warn("idempotency: marshal cached response failed", zap.Error(err))
		_ = client.Del(ctx, redisKey).Err()
		return
	}

	if err := client.Set(ctx, redisKey, string(payload), ttl).Err(); err != nil {
		logger.Warn("idempotency: cache response failed", zap.Error(err), zap.String("key", redisKey))
	}
}

func shouldCache(status int, body []byte) bool {
	if status >= 500 {
		return false
	}

	var env codeEnvelope
	if err := json.Unmarshal(body, &env); err != nil || env.Code == nil {
		return false
	}
	return *env.Code < errs.CodeInternal
}

type bodyCaptureWriter struct {
	gin.ResponseWriter
	buf *bytes.Buffer
}

func (w *bodyCaptureWriter) Write(b []byte) (int, error) {
	w.buf.Write(b)
	return w.ResponseWriter.Write(b)
}

func (w *bodyCaptureWriter) WriteString(s string) (int, error) {
	w.buf.WriteString(s)
	return w.ResponseWriter.WriteString(s)
}
