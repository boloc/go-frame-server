package idempotency

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/boloc/go-frame-server/pkg/errs"
	"github.com/boloc/go-frame-server/pkg/frame/webx"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// fakeRedis 是只实现 SetNX/Get/Set/Del 的内存 redis.Cmdable。
type fakeRedis struct {
	redis.Cmdable

	mu    sync.Mutex
	store map[string]string
}

func newFakeRedis() *fakeRedis {
	return &fakeRedis{store: make(map[string]string)}
}

func (f *fakeRedis) SetNX(_ context.Context, key string, value any, _ time.Duration) *redis.BoolCmd {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, exists := f.store[key]; exists {
		return redis.NewBoolResult(false, nil)
	}
	f.store[key] = value.(string)
	return redis.NewBoolResult(true, nil)
}

func (f *fakeRedis) Get(_ context.Context, key string) *redis.StringCmd {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.store[key]
	if !ok {
		return redis.NewStringResult("", redis.Nil)
	}
	return redis.NewStringResult(v, nil)
}

func (f *fakeRedis) Set(_ context.Context, key string, value any, _ time.Duration) *redis.StatusCmd {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.store[key] = value.(string)
	return redis.NewStatusResult("OK", nil)
}

func (f *fakeRedis) Del(_ context.Context, keys ...string) *redis.IntCmd {
	f.mu.Lock()
	defer f.mu.Unlock()
	var n int64
	for _, k := range keys {
		if _, ok := f.store[k]; ok {
			delete(f.store, k)
			n++
		}
	}
	return redis.NewIntResult(n, nil)
}

func newTestEngine(t *testing.T, redisClient *fakeRedis, opts ...Option) (*gin.Engine, *int32) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()

	var calls int32
	allOpts := append([]Option{WithRedis(func() (redis.Cmdable, bool) { return redisClient, true })}, opts...)
	engine.Use(Middleware(allOpts...))

	engine.POST("/orders", func(c *gin.Context) {
		calls++
		switch c.Query("scenario") {
		case "business_error":
			webx.Fail(c, errs.Conflict("库存不足"))
		case "internal_error":
			webx.Fail(c, errs.Internal(context.DeadlineExceeded))
		default:
			webx.Success(c, gin.H{"order_id": calls})
		}
	})
	return engine, &calls
}

func doRequest(engine *gin.Engine, idempotencyKey, scenario string) *httptest.ResponseRecorder {
	url := "/orders"
	if scenario != "" {
		url += "?scenario=" + scenario
	}
	req := httptest.NewRequest(http.MethodPost, url, nil)
	if idempotencyKey != "" {
		req.Header.Set(DefaultHeader, idempotencyKey)
	}
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

// TestMiddlewareWithoutKeyDefaultsToPassThrough 验证未要求必传时缺少幂等键直接放行。
func TestMiddlewareWithoutKeyDefaultsToPassThrough(t *testing.T) {
	engine, calls := newTestEngine(t, newFakeRedis())

	w := doRequest(engine, "", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if *calls != 1 {
		t.Fatalf("handler 应该被调用一次，实际 %d 次", *calls)
	}
}

// TestMiddlewareRequiredRejectsMissingKey 验证 Required=true 时缺少幂等键被拒绝。
func TestMiddlewareRequiredRejectsMissingKey(t *testing.T) {
	engine, calls := newTestEngine(t, newFakeRedis(), WithRequired(true))

	w := doRequest(engine, "", "")
	if *calls != 0 {
		t.Fatalf("缺少幂等键时 handler 不应该被调用，实际调用了 %d 次", *calls)
	}
	if w.Code != http.StatusOK { // webx.Fail 固定 200，真正的错误码在 body.code 里
		t.Fatalf("status = %d, want 200（webx.Fail 约定）", w.Code)
	}
}

// TestMiddlewareReplaysCachedSuccessResponse 验证同一幂等键第二次请求重放首次成功响应。
func TestMiddlewareReplaysCachedSuccessResponse(t *testing.T) {
	engine, calls := newTestEngine(t, newFakeRedis())

	first := doRequest(engine, "order-key-1", "")
	second := doRequest(engine, "order-key-1", "")

	if *calls != 1 {
		t.Fatalf("handler 应该只被执行一次，实际执行了 %d 次", *calls)
	}
	if first.Code != http.StatusOK || second.Code != http.StatusOK {
		t.Fatalf("两次响应状态码都应该是 200，实际 %d / %d", first.Code, second.Code)
	}
	if first.Body.String() != second.Body.String() {
		t.Fatalf("重放的响应体应该和第一次完全一致：\n第一次: %s\n第二次: %s", first.Body.String(), second.Body.String())
	}
}

// TestMiddlewareCachesDefiniteBusinessError 验证明确的业务错误也会被缓存并重放。
func TestMiddlewareCachesDefiniteBusinessError(t *testing.T) {
	engine, calls := newTestEngine(t, newFakeRedis())

	first := doRequest(engine, "order-key-2", "business_error")
	second := doRequest(engine, "order-key-2", "business_error")

	if *calls != 1 {
		t.Fatalf("业务错误也应该被缓存，handler 应该只执行一次，实际执行了 %d 次", *calls)
	}
	if first.Body.String() != second.Body.String() {
		t.Fatalf("重放的业务错误响应应该和第一次一致")
	}
}

// TestMiddlewareReleasesLockOnInternalError 验证内部错误会释放锁，同一 key 可重试执行。
func TestMiddlewareReleasesLockOnInternalError(t *testing.T) {
	engine, calls := newTestEngine(t, newFakeRedis())

	first := doRequest(engine, "order-key-3", "internal_error")
	if first.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", first.Code)
	}

	// 内部错误发生后，用同一个 key 换一个正常场景重试，应该真正执行到 handler（不是重放）。
	second := doRequest(engine, "order-key-3", "")
	if *calls != 2 {
		t.Fatalf("内部错误之后应该释放锁允许重试，handler 应该被执行 2 次，实际 %d 次", *calls)
	}
	if second.Code != http.StatusOK {
		t.Fatalf("重试响应 status = %d, want 200", second.Code)
	}
}

// TestMiddlewareRejectsConcurrentSameKeyBeforeFirstCompletes 验证处理中的同一幂等键会被拒绝。
func TestMiddlewareRejectsConcurrentSameKeyBeforeFirstCompletes(t *testing.T) {
	client := newFakeRedis()
	// 手动模拟"正在处理中"：SETNX 已经抢占，但还没到 finalize 阶段。
	ctx := context.Background()
	client.SetNX(ctx, DefaultKeyPrefix+"/orders:order-key-4", processingMarker, time.Hour)

	engine, calls := newTestEngine(t, client)
	w := doRequest(engine, "order-key-4", "")

	if *calls != 0 {
		t.Fatalf("正在处理中的幂等键不应该触发 handler 执行，实际执行了 %d 次", *calls)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200（webx.Fail 约定）", w.Code)
	}
}

// TestMiddlewareFallsBackWhenRedisUnavailable 验证 Redis 不可用时默认 fail-open 放行。
func TestMiddlewareFallsBackWhenRedisUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	var calls int32
	engine.Use(Middleware(WithRedis(func() (redis.Cmdable, bool) { return nil, false })))
	engine.POST("/orders", func(c *gin.Context) {
		calls++
		webx.Success(c, gin.H{"ok": true})
	})

	w := doRequest(engine, "order-key-5", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if calls != 1 {
		t.Fatalf("Redis 不可用时应该降级放行，handler 应该被调用一次，实际 %d 次", calls)
	}
}

// TestMiddlewareFailOpenFalseRejectsWhenRedisUnavailable 验证 FailOpen=false 时 Redis 不可用直接拒绝。
func TestMiddlewareFailOpenFalseRejectsWhenRedisUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	var calls int32
	engine.Use(Middleware(
		WithRedis(func() (redis.Cmdable, bool) { return nil, false }),
		WithFailOpen(false),
	))
	engine.POST("/orders", func(c *gin.Context) {
		calls++
		webx.Success(c, gin.H{"ok": true})
	})

	w := doRequest(engine, "order-key-6", "")
	if calls != 0 {
		t.Fatalf("FailOpen=false 时 Redis 不可用应该直接拒绝，handler 不应该被调用，实际调用了 %d 次", calls)
	}
	if w.Code != http.StatusOK { // webx.Fail 固定 200，真正的错误码在 body.code 里
		t.Fatalf("status = %d, want 200（webx.Fail 约定）", w.Code)
	}
}

// TestMiddlewarePanicReleasesLockForRetry 验证 handler panic 时幂等锁会被释放，
// 同一个 key 之后可以重试。
func TestMiddlewarePanicReleasesLockForRetry(t *testing.T) {
	client := newFakeRedis()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(gin.Recovery()) // 跟真实的 components.NewGinComponent 保持一致
	var calls int32
	engine.Use(Middleware(WithRedis(func() (redis.Cmdable, bool) { return client, true })))
	engine.POST("/orders", func(c *gin.Context) {
		calls++
		if calls == 1 {
			panic("演示用：第一次调用故意 panic")
		}
		webx.Success(c, gin.H{"order_id": calls})
	})

	first := doRequest(engine, "order-key-panic", "")
	if first.Code != http.StatusInternalServerError {
		t.Fatalf("第一次请求 status = %d, want 500（gin.Recovery() 恢复 panic 后的默认状态码）", first.Code)
	}

	second := doRequest(engine, "order-key-panic", "")
	if calls != 2 {
		t.Fatalf("panic 之后幂等锁应该被释放，同一个 key 重试应该真正执行到 handler，实际执行了 %d 次", calls)
	}
	if second.Code != http.StatusOK {
		t.Fatalf("重试响应 status = %d, want 200", second.Code)
	}
}

// TestShouldCacheRejectsBodyWithoutCodeField 验证不带 code 字段的响应体（非 webx
// 格式）不会被误判成成功并缓存。
func TestShouldCacheRejectsBodyWithoutCodeField(t *testing.T) {
	if shouldCache(http.StatusOK, []byte(`{"data":{"foo":"bar"}}`)) {
		t.Fatal("不带 code 字段的响应体不应该被当成可缓存的成功响应")
	}
	if shouldCache(http.StatusOK, []byte(`[]`)) {
		t.Fatal("非 webx 格式（数组）的响应体不应该被当成可缓存的成功响应")
	}
	if !shouldCache(http.StatusOK, []byte(`{"code":0,"message":"success"}`)) {
		t.Fatal("显式带 code:0（CodeOK）的响应体应该被当成可缓存的成功响应")
	}
}

// TestMiddlewareDifferentRoutesDoNotShareKeySpace 验证不同路由的同名幂等键互不影响。
func TestMiddlewareDifferentRoutesDoNotShareKeySpace(t *testing.T) {
	client := newFakeRedis()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	var ordersCalls, refundsCalls int32
	engine.Use(Middleware(WithRedis(func() (redis.Cmdable, bool) { return client, true })))
	engine.POST("/orders", func(c *gin.Context) {
		ordersCalls++
		webx.Success(c, gin.H{"n": ordersCalls})
	})
	engine.POST("/refunds", func(c *gin.Context) {
		refundsCalls++
		webx.Success(c, gin.H{"n": refundsCalls})
	})

	req1 := httptest.NewRequest(http.MethodPost, "/orders", nil)
	req1.Header.Set(DefaultHeader, "shared-key")
	engine.ServeHTTP(httptest.NewRecorder(), req1)

	req2 := httptest.NewRequest(http.MethodPost, "/refunds", nil)
	req2.Header.Set(DefaultHeader, "shared-key")
	engine.ServeHTTP(httptest.NewRecorder(), req2)

	if ordersCalls != 1 || refundsCalls != 1 {
		t.Fatalf("两个不同路由复用同一个幂等键不应该互相影响，实际 orders=%d refunds=%d", ordersCalls, refundsCalls)
	}
}
