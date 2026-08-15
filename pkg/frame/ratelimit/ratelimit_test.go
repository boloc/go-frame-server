package ratelimit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/boloc/go-frame-server/pkg/frame/middleware"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// fakeRedis 是只实现 Eval 的内存计数器，用于测中间件决策，不模拟真实过期。
type fakeRedis struct {
	redis.Cmdable

	mu     sync.Mutex
	counts map[string]int64
	evalErr error
}

func newFakeRedis() *fakeRedis {
	return &fakeRedis{counts: make(map[string]int64)}
}

func (f *fakeRedis) Eval(_ context.Context, _ string, keys []string, _ ...any) *redis.Cmd {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.evalErr != nil {
		return redis.NewCmdResult(nil, f.evalErr)
	}
	key := keys[0]
	f.counts[key]++
	return redis.NewCmdResult(f.counts[key], nil)
}

func (f *fakeRedis) reset(key string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.counts, key)
}

func newTestEngine(client *fakeRedis, opts ...Option) (*gin.Engine, *int32) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(middleware.ContextMiddleware()) // defaultKeyFunc 依赖 reqctx.ClientIP

	var calls int32
	allOpts := append([]Option{WithRedis(func() (redis.Cmdable, bool) { return client, true })}, opts...)
	engine.Use(Middleware(allOpts...))
	engine.GET("/orders", func(c *gin.Context) {
		calls++
		c.String(http.StatusOK, "ok")
	})
	return engine, &calls
}

func doGet(engine *gin.Engine) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/orders", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

// TestMiddlewarePanicsWithoutLimitOrWindow 验证未设置 Limit/Window 时 Middleware 立即 panic。
func TestMiddlewarePanicsWithoutLimitOrWindow(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("Limit/Window 都没设置时 Middleware 应该 panic")
		}
	}()
	Middleware()
}

// TestMiddlewareAllowsRequestsWithinLimit 验证窗口内未超限的请求全部放行。
func TestMiddlewareAllowsRequestsWithinLimit(t *testing.T) {
	client := newFakeRedis()
	engine, calls := newTestEngine(client, WithLimit(3), WithWindow(time.Minute))

	for i := 0; i < 3; i++ {
		w := doGet(engine)
		if w.Code != http.StatusOK {
			t.Fatalf("第 %d 次请求 status = %d, want 200", i+1, w.Code)
		}
	}
	if *calls != 3 {
		t.Fatalf("handler 应该被调用 3 次，实际 %d 次", *calls)
	}
}

// TestMiddlewareRejectsRequestsOverLimit 验证超限后拒绝请求且不再调用 handler。
func TestMiddlewareRejectsRequestsOverLimit(t *testing.T) {
	client := newFakeRedis()
	engine, calls := newTestEngine(client, WithLimit(2), WithWindow(time.Minute))

	doGet(engine)
	doGet(engine)
	third := doGet(engine)

	if *calls != 2 {
		t.Fatalf("超限之后 handler 不应该再被调用，实际总共调用了 %d 次", *calls)
	}
	if third.Code != http.StatusTooManyRequests {
		t.Fatalf("超限响应 status = %d, want 429（UseRealStatus 默认 true）", third.Code)
	}
	if third.Header().Get("Retry-After") == "" {
		t.Fatal("超限响应应该带 Retry-After 头")
	}
}

// TestMiddlewareUseRealStatusFalseRespondsWith200 验证 UseRealStatus=false 时超限仍返回 HTTP 200。
func TestMiddlewareUseRealStatusFalseRespondsWith200(t *testing.T) {
	client := newFakeRedis()
	engine, _ := newTestEngine(client, WithLimit(1), WithWindow(time.Minute), WithUseRealStatus(false))

	doGet(engine)
	second := doGet(engine)

	if second.Code != http.StatusOK {
		t.Fatalf("UseRealStatus=false 时 status = %d, want 200（webx.Fail 约定）", second.Code)
	}
}

// TestMiddlewareFailOpenDefaultAllowsWhenRedisErrors 验证默认 FailOpen 下 Redis 出错时降级放行。
func TestMiddlewareFailOpenDefaultAllowsWhenRedisErrors(t *testing.T) {
	client := newFakeRedis()
	client.evalErr = context.DeadlineExceeded
	engine, calls := newTestEngine(client, WithLimit(1), WithWindow(time.Minute))

	w := doGet(engine)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if *calls != 1 {
		t.Fatalf("FailOpen=true 时 EVAL 出错应该降级放行，handler 应该被调用，实际调用了 %d 次", *calls)
	}
}

// TestMiddlewareFailOpenFalseRejectsWhenRedisErrors 验证 FailOpen=false 时 Redis 出错直接拒绝。
func TestMiddlewareFailOpenFalseRejectsWhenRedisErrors(t *testing.T) {
	client := newFakeRedis()
	client.evalErr = context.DeadlineExceeded
	engine, calls := newTestEngine(client, WithLimit(1), WithWindow(time.Minute), WithFailOpen(false))

	w := doGet(engine)
	if *calls != 0 {
		t.Fatalf("FailOpen=false 时 EVAL 出错应该拒绝，handler 不应该被调用，实际调用了 %d 次", *calls)
	}
	if w.Code != http.StatusOK { // webx.Fail 固定 200
		t.Fatalf("status = %d, want 200（webx.Fail 约定）", w.Code)
	}
}

// TestMiddlewareDifferentClientIPsHaveIndependentLimits 验证不同 ClientIP 有独立限流计数。
func TestMiddlewareDifferentClientIPsHaveIndependentLimits(t *testing.T) {
	client := newFakeRedis()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(middleware.ContextMiddleware())
	engine.Use(Middleware(WithRedis(func() (redis.Cmdable, bool) { return client, true }), WithLimit(1), WithWindow(time.Minute)))
	engine.GET("/orders", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	req1 := httptest.NewRequest(http.MethodGet, "/orders", nil)
	req1.RemoteAddr = "1.1.1.1:1234"
	w1 := httptest.NewRecorder()
	engine.ServeHTTP(w1, req1)

	req2 := httptest.NewRequest(http.MethodGet, "/orders", nil)
	req2.RemoteAddr = "2.2.2.2:5678"
	w2 := httptest.NewRecorder()
	engine.ServeHTTP(w2, req2)

	if w1.Code != http.StatusOK || w2.Code != http.StatusOK {
		t.Fatalf("两个不同 IP 各自的第一次请求都应该放行，实际 %d / %d", w1.Code, w2.Code)
	}
}

// TestMiddlewareWithCustomKeyFunc 验证 WithKeyFunc 按自定义维度独立限流。
func TestMiddlewareWithCustomKeyFunc(t *testing.T) {
	client := newFakeRedis()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(Middleware(
		WithRedis(func() (redis.Cmdable, bool) { return client, true }),
		WithLimit(1),
		WithWindow(time.Minute),
		WithKeyFunc(func(c *gin.Context) string { return c.Query("user_id") }),
	))
	engine.GET("/orders", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	req1 := httptest.NewRequest(http.MethodGet, "/orders?user_id=1", nil)
	w1 := httptest.NewRecorder()
	engine.ServeHTTP(w1, req1)

	req2 := httptest.NewRequest(http.MethodGet, "/orders?user_id=2", nil)
	w2 := httptest.NewRecorder()
	engine.ServeHTTP(w2, req2)

	if w1.Code != http.StatusOK || w2.Code != http.StatusOK {
		t.Fatalf("两个不同 user_id 各自的第一次请求都应该放行，实际 %d / %d", w1.Code, w2.Code)
	}
}
