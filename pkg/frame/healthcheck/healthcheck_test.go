package healthcheck

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

func newRouterWithHandler(h gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/health", h)
	return r
}

func TestHandlerHealthyWhenAllChecksPass(t *testing.T) {
	h := Handler([]Dependency{
		{Name: "ok-dep", Critical: true, Check: func(ctx context.Context) error { return nil }},
	})

	r := newRouterWithHandler(h)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestHandlerUnhealthyWhenCriticalCheckFails(t *testing.T) {
	h := Handler([]Dependency{
		{Name: "bad-dep", Critical: true, Check: func(ctx context.Context) error { return errors.New("down") }},
	})

	r := newRouterWithHandler(h)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}

// TestNonCriticalFailureDoesNotAffectOverallHealth 验证非核心依赖失败不影响整体健康状态。
func TestNonCriticalFailureDoesNotAffectOverallHealth(t *testing.T) {
	h := Handler([]Dependency{
		{Name: "critical-ok", Critical: true, Check: func(ctx context.Context) error { return nil }},
		{Name: "noncritical-bad", Critical: false, Check: func(ctx context.Context) error { return errors.New("down") }},
	})

	r := newRouterWithHandler(h)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200（非核心依赖失败不应该影响整体健康状态）", w.Code)
	}
}

// TestResultIsCached 验证缓存 TTL 内不会重复执行 Checker。
func TestResultIsCached(t *testing.T) {
	calls := 0
	h := Handler([]Dependency{
		{Name: "dep", Critical: true, Check: func(ctx context.Context) error {
			calls++
			return nil
		}},
	}, WithCacheTTL(time.Minute))

	r := newRouterWithHandler(h)
	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))
	}

	if calls != 1 {
		t.Fatalf("Checker 被调用了 %d 次，want 1（后续请求应该命中缓存）", calls)
	}
}

// TestPerCheckTimeoutBoundsSlowChecker 验证慢检查会在 PerCheckTimeout 后超时结束。
func TestPerCheckTimeoutBoundsSlowChecker(t *testing.T) {
	h := Handler([]Dependency{
		{Name: "slow-dep", Critical: true, Check: func(ctx context.Context) error {
			<-ctx.Done() // 故意一直卡到 ctx 超时才返回
			return ctx.Err()
		}},
	}, WithPerCheckTimeout(50*time.Millisecond))

	r := newRouterWithHandler(h)
	start := time.Now()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))
	elapsed := time.Since(start)

	if elapsed > time.Second {
		t.Fatalf("elapsed = %v，PerCheckTimeout 应该让检查很快结束，不是卡住不动", elapsed)
	}
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}

// TestMySQLCheckerReturnsErrorWhenNotInitialized 验证 DB 未初始化时 MySQLChecker 返回 error，不 panic。
func TestMySQLCheckerReturnsErrorWhenNotInitialized(t *testing.T) {
	checker := MySQLChecker(func() (*gorm.DB, bool) {
		return nil, false
	})

	err := checker(context.Background())
	if err == nil {
		t.Fatal("expected an error when the underlying db is not initialized")
	}
}

// TestRedisCheckerReturnsErrorWhenNotInitialized 验证 Redis 未初始化时 RedisChecker 返回 error，不 panic。
func TestRedisCheckerReturnsErrorWhenNotInitialized(t *testing.T) {
	checker := RedisChecker(func() (redis.Cmdable, bool) {
		return nil, false
	})

	err := checker(context.Background())
	if err == nil {
		t.Fatal("expected an error when the underlying redis client is not initialized")
	}
}
