// Package healthcheck 提供进程存活（liveness）与依赖就绪（readiness）检查。
//
// Handler 是 readiness 语义：检查 MySQL/Redis 等依赖，结果短 TTL 缓存，每个依赖单独超时。
// 把它接到 k8s readinessProbe——依赖挂了摘流量。不要把 Handler 接到 livenessProbe 上，
// 否则 Redis 一挂所有 Pod 会被循环重启。
//
// Liveness 只表示进程还活着，不检查任何依赖、不做缓存，接到 livenessProbe。
package healthcheck

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"
)

// Checker 是一次具体依赖的健康检查：ctx 带超时，返回 nil 表示健康。
type Checker func(ctx context.Context) error

// Dependency 一个命名的健康检查项。
type Dependency struct {
	Name string
	// Check 具体的检查逻辑，通常用本包提供的 MySQLChecker/RedisChecker 构造。
	Check Checker
	// Critical 为 false 时，该项失败不影响整体 healthy。
	Critical bool
}

// Options 控制 Handler 的行为。
type Options struct {
	PerCheckTimeout time.Duration
	CacheTTL        time.Duration
}

// WithPerCheckTimeout 设置每个依赖检查的超时时间，默认 1s。
func WithPerCheckTimeout(d time.Duration) func(*Options) {
	return func(o *Options) { o.PerCheckTimeout = d }
}

// WithCacheTTL 设置结果缓存时间，默认 10s。
func WithCacheTTL(d time.Duration) func(*Options) {
	return func(o *Options) { o.CacheTTL = d }
}

type checkResult struct {
	Status    string   `json:"status"` // ok / error
	LatencyMs *float64 `json:"latency_ms,omitempty"`
	Error     string   `json:"error,omitempty"`
}

type cachedResult struct {
	expiresAt time.Time
	healthy   bool
	checks    map[string]checkResult
}

// Liveness 返回进程存活探针：进程活着就 200 {"status":"ok"}，不检查任何依赖，不做缓存。
// 接到 k8s livenessProbe（/livez）。不要用 Handler 充当 liveness。
func Liveness() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
}

// Handler 返回 readiness 探针：检查依赖连通性，结果短 TTL 缓存。
// 接到 k8s readinessProbe（/readyz、/health）。不要把它接到 livenessProbe 上，
// 否则 Redis 一挂所有 Pod 会被循环重启。
//
//	readyz := healthcheck.Handler([]healthcheck.Dependency{
//	    {Name: "mysql", Critical: true, Check: healthcheck.MySQLChecker(frame.TryDefaultDB)},
//	    {Name: "redis", Critical: true, Check: healthcheck.RedisChecker(frame.TryGetRedisCmdable)},
//	})
//	r.GET("/health", readyz)
//	r.GET("/readyz", readyz)
//	r.GET("/livez", healthcheck.Liveness())
func Handler(deps []Dependency, opts ...func(*Options)) gin.HandlerFunc {
	o := &Options{PerCheckTimeout: time.Second, CacheTTL: 10 * time.Second}
	for _, opt := range opts {
		opt(o)
	}

	var last atomic.Pointer[cachedResult]
	var sf singleflight.Group

	return func(c *gin.Context) {
		if cached := last.Load(); cached != nil && time.Now().Before(cached.expiresAt) {
			writeResponse(c, cached.healthy, cached.checks)
			return
		}

		v, _, _ := sf.Do("readyz", func() (any, error) {
			if cached := last.Load(); cached != nil && time.Now().Before(cached.expiresAt) {
				return cached, nil
			}

			checks := make(map[string]checkResult, len(deps))
			healthy := true
			for _, dep := range deps {
				res := runCheck(dep.Check, o.PerCheckTimeout)
				checks[dep.Name] = res
				if res.Status == "error" && dep.Critical {
					healthy = false
				}
			}

			result := &cachedResult{
				expiresAt: time.Now().Add(o.CacheTTL),
				healthy:   healthy,
				checks:    checks,
			}
			last.Store(result)
			return result, nil
		})
		cached := v.(*cachedResult)
		writeResponse(c, cached.healthy, cached.checks)
	}
}

func runCheck(check Checker, timeout time.Duration) checkResult {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	start := time.Now()
	err := check(ctx)
	ms := float64(time.Since(start).Microseconds()) / 1000

	if err != nil {
		return checkResult{Status: "error", LatencyMs: &ms, Error: err.Error()}
	}
	return checkResult{Status: "ok", LatencyMs: &ms}
}

func writeResponse(c *gin.Context, healthy bool, checks map[string]checkResult) {
	status := http.StatusOK
	if !healthy {
		status = http.StatusServiceUnavailable
	}
	c.JSON(status, gin.H{
		"healthy": healthy,
		"checks":  checks,
	})
}

// MySQLChecker 检查 *gorm.DB 连通性。getDB 应使用 Try 语义的访问函数。
func MySQLChecker(getDB func() (*gorm.DB, bool)) Checker {
	return func(ctx context.Context) error {
		db, ok := getDB()
		if !ok || db == nil {
			return errors.New("db not initialized")
		}
		sqlDB, err := db.DB()
		if err != nil {
			return err
		}
		return sqlDB.PingContext(ctx)
	}
}

// RedisChecker 检查 Redis 连通性。getClient 应使用 Try 语义的访问函数。
func RedisChecker(getClient func() (redis.Cmdable, bool)) Checker {
	return func(ctx context.Context) error {
		client, ok := getClient()
		if !ok || client == nil {
			return errors.New("redis client not initialized")
		}
		return client.Ping(ctx).Err()
	}
}
