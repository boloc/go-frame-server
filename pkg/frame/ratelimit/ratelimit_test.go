package ratelimit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/boloc/go-frame-server/v2/pkg/frame/rediskey"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func TestMain(m *testing.M) {
	rediskey.SetNamespace("ratelimit-test")
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

// fakeClock 同时推进 miniredis 的 TIME 和 key 过期。
type fakeClock struct {
	mr  *miniredis.Miniredis
	now time.Time
}

func (c *fakeClock) advance(d time.Duration) {
	c.now = c.now.Add(d)
	c.mr.SetTime(c.now)
	c.mr.FastForward(d)
}

func newMiniLimiter(t *testing.T) (*Limiter, *fakeClock) {
	t.Helper()
	mr := miniredis.RunT(t)
	start := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	mr.SetTime(start)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return New(func() (redis.Cmdable, bool) { return client, true }), &fakeClock{mr: mr, now: start}
}

func mustAllow(t *testing.T, l *Limiter, rule Rule, key string) Result {
	t.Helper()
	res, err := l.Allow(context.Background(), rule, key)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestSmoothBurstThenSteadyRate(t *testing.T) {
	l, clk := newMiniLimiter(t)
	rule := Smooth("smooth", 60, time.Minute, 5) // 每秒 1 次，最多连续 5 次

	for i := 4; i >= 0; i-- {
		res := mustAllow(t, l, rule, "ip")
		if !res.Allowed || res.Remaining != i || res.Limit != 5 {
			t.Fatalf("burst request: %+v, want remaining %d", res, i)
		}
	}
	res := mustAllow(t, l, rule, "ip")
	if res.Allowed || res.RetryAfter != time.Second {
		t.Fatalf("over burst: %+v", res)
	}

	clk.advance(time.Second)
	if res := mustAllow(t, l, rule, "ip"); !res.Allowed {
		t.Fatalf("after 1s: %+v", res)
	}
	if res := mustAllow(t, l, rule, "ip"); res.Allowed {
		t.Fatalf("second request after 1s should wait: %+v", res)
	}

	clk.advance(time.Minute)
	for i := 0; i < 5; i++ {
		if res := mustAllow(t, l, rule, "ip"); !res.Allowed {
			t.Fatalf("refilled burst #%d: %+v", i, res)
		}
	}
	if res := mustAllow(t, l, rule, "ip"); res.Allowed {
		t.Fatalf("refill is capped at burst: %+v", res)
	}
}

// 固定窗口在边界前后各打满一次，两秒内能打出 2×limit。平滑限流打满之后只能按速率慢慢恢复。
func TestSmoothHasNoWindowBoundary(t *testing.T) {
	l, clk := newMiniLimiter(t)
	rule := Smooth("boundary", 10, time.Minute, 10) // 每 6 秒恢复 1 次

	clk.advance(59 * time.Second)
	for i := 0; i < 10; i++ {
		if res := mustAllow(t, l, rule, "ip"); !res.Allowed {
			t.Fatalf("first burst #%d: %+v", i, res)
		}
	}
	clk.advance(2 * time.Second)
	res := mustAllow(t, l, rule, "ip")
	if res.Allowed {
		t.Fatalf("2s after a full burst nothing has refilled yet: %+v", res)
	}
	if res.RetryAfter != 4*time.Second {
		t.Fatalf("retry after %s, want 4s", res.RetryAfter)
	}
}

func TestSlidingIsStrictInAnyWindow(t *testing.T) {
	l, clk := newMiniLimiter(t)
	rule := Sliding("sliding", 3, time.Minute)

	for _, step := range []time.Duration{0, 10 * time.Second, 10 * time.Second} {
		clk.advance(step)
		if res := mustAllow(t, l, rule, "ip"); !res.Allowed {
			t.Fatalf("within limit: %+v", res)
		}
	}

	clk.advance(10 * time.Second) // t=30s
	res := mustAllow(t, l, rule, "ip")
	if res.Allowed || res.RetryAfter != 30*time.Second || res.Remaining != 0 {
		t.Fatalf("4th in window: %+v", res)
	}

	clk.advance(30*time.Second - time.Millisecond) // t=59.999s
	if res := mustAllow(t, l, rule, "ip"); res.Allowed || res.RetryAfter != time.Millisecond {
		t.Fatalf("1ms before the oldest leaves: %+v", res)
	}

	clk.advance(time.Millisecond) // t=60s，t=0 那条移出窗口
	if res := mustAllow(t, l, rule, "ip"); !res.Allowed {
		t.Fatalf("oldest left the window: %+v", res)
	}
	res = mustAllow(t, l, rule, "ip")
	if res.Allowed || res.RetryAfter != 10*time.Second {
		t.Fatalf("next slot opens when t=10s leaves: %+v", res)
	}
}

func TestBanOutlastsTheRate(t *testing.T) {
	for _, rule := range []Rule{
		Sliding("ban-sliding", 2, time.Minute).WithBan(banOnce(10 * time.Minute)),
		Smooth("ban-smooth", 2, time.Minute, 2).WithBan(banOnce(10 * time.Minute)),
		Fixed("ban-fixed", 2, time.Minute).WithBan(banOnce(10 * time.Minute)),
	} {
		t.Run(rule.Name(), func(t *testing.T) {
			l, clk := newMiniLimiter(t)
			mustAllow(t, l, rule, "ip")
			mustAllow(t, l, rule, "ip")

			res := mustAllow(t, l, rule, "ip")
			if res.Allowed || !res.Banned || res.RetryAfter != 10*time.Minute {
				t.Fatalf("exceeding should ban: %+v", res)
			}

			clk.advance(2 * time.Minute)
			res = mustAllow(t, l, rule, "ip")
			if res.Allowed || !res.Banned || res.RetryAfter != 8*time.Minute {
				t.Fatalf("still banned after the rate recovered: %+v", res)
			}

			clk.advance(8 * time.Minute)
			if res := mustAllow(t, l, rule, "ip"); !res.Allowed || res.Banned {
				t.Fatalf("ban expired: %+v", res)
			}
		})
	}
}

// 一个计数累加：从第一次请求起算一轮，满了等这一轮结束，结束后从 0 重新算。
func TestFixedWindow(t *testing.T) {
	l, clk := newMiniLimiter(t)
	rule := Fixed("fixed", 3, time.Minute)

	for want := 2; want >= 0; want-- {
		if res := mustAllow(t, l, rule, "ip"); !res.Allowed || res.Remaining != want {
			t.Fatalf("within limit: %+v, want remaining %d", res, want)
		}
	}
	res := mustAllow(t, l, rule, "ip")
	if res.Allowed || res.RetryAfter != time.Minute {
		t.Fatalf("4th in the round: %+v", res)
	}

	clk.advance(40 * time.Second)
	if res := mustAllow(t, l, rule, "ip"); res.Allowed || res.RetryAfter != 20*time.Second {
		t.Fatalf("still the same round: %+v", res)
	}

	clk.advance(20 * time.Second)
	if res := mustAllow(t, l, rule, "ip"); !res.Allowed || res.Remaining != 2 || res.ResetAfter != time.Minute {
		t.Fatalf("new round starts from 0: %+v", res)
	}
}

// 第一次被拒就封 d，只有一档。
func banOnce(d time.Duration) Ban {
	return Ban{Strikes: 1, Durations: []time.Duration{d}, Forget: d}
}

// 正常用户偶尔被拒一两次不该被封；持续被拒才封。
func TestBanNeedsStrikesWithinWindow(t *testing.T) {
	l, clk := newMiniLimiter(t)
	rule := Sliding("strikes", 1, time.Hour).WithBan(Ban{
		Strikes: 3, Within: 10 * time.Second,
		Durations: []time.Duration{time.Minute}, Forget: time.Hour,
	})
	mustAllow(t, l, rule, "ip")

	for i := 1; i <= 2; i++ {
		if res := mustAllow(t, l, rule, "ip"); res.Allowed || res.Banned {
			t.Fatalf("strike %d should only be rejected: %+v", i, res)
		}
	}
	clk.advance(11 * time.Second) // 前两次被拒移出 Within
	for i := 1; i <= 2; i++ {
		if res := mustAllow(t, l, rule, "ip"); res.Banned {
			t.Fatalf("old strikes were forgotten, strike %d: %+v", i, res)
		}
	}
	if res := mustAllow(t, l, rule, "ip"); !res.Banned || res.RetryAfter != time.Minute {
		t.Fatalf("third strike within 10s bans: %+v", res)
	}
}

func TestBanEscalatesAndForgets(t *testing.T) {
	l, clk := newMiniLimiter(t)
	rule := Sliding("escalate", 1, time.Second).WithBan(Ban{
		Strikes: 1, Durations: []time.Duration{time.Minute, 10 * time.Minute}, Forget: time.Hour,
	})
	exceed := func() Result {
		t.Helper()
		mustAllow(t, l, rule, "ip")
		return mustAllow(t, l, rule, "ip")
	}

	for i, want := range []time.Duration{time.Minute, 10 * time.Minute, 10 * time.Minute} {
		res := exceed()
		if !res.Banned || res.RetryAfter != want {
			t.Fatalf("ban #%d: %+v, want %s", i+1, res, want)
		}
		clk.advance(want)
	}

	clk.advance(time.Hour) // 一小时没再被封
	if res := exceed(); !res.Banned || res.RetryAfter != time.Minute {
		t.Fatalf("after Forget it starts from the first step: %+v", res)
	}
}

func TestStatusDoesNotConsume(t *testing.T) {
	ctx := context.Background()
	for _, rule := range []Rule{
		Sliding("status-sliding", 2, time.Minute),
		Smooth("status-smooth", 2, time.Minute, 2),
		Fixed("status-fixed", 2, time.Minute),
	} {
		t.Run(rule.Name(), func(t *testing.T) {
			l, _ := newMiniLimiter(t)
			for want := 2; want > 0; want-- {
				res, err := l.Status(ctx, rule, "account")
				if err != nil {
					t.Fatal(err)
				}
				if !res.Allowed || res.Remaining != want {
					t.Fatalf("status: %+v, want remaining %d", res, want)
				}
				mustAllow(t, l, rule, "account")
			}
			res, err := l.Status(ctx, rule, "account")
			if err != nil {
				t.Fatal(err)
			}
			if res.Allowed || res.Remaining != 0 || res.RetryAfter <= 0 {
				t.Fatalf("exhausted status: %+v", res)
			}
		})
	}
}

func TestStatusDoesNotTriggerBan(t *testing.T) {
	ctx := context.Background()
	l, _ := newMiniLimiter(t)
	rule := Sliding("status-ban", 1, time.Minute).WithBan(banOnce(time.Hour))
	mustAllow(t, l, rule, "account")

	res, err := l.Status(ctx, rule, "account")
	if err != nil {
		t.Fatal(err)
	}
	if res.Allowed || res.Banned {
		t.Fatalf("status must not ban: %+v", res)
	}
	if res := mustAllow(t, l, rule, "account"); !res.Banned {
		t.Fatalf("allow over the limit bans: %+v", res)
	}
}

func TestResetClearsCounterAndBan(t *testing.T) {
	ctx := context.Background()
	l, _ := newMiniLimiter(t)
	rule := Sliding("reset", 1, time.Hour).WithBan(Ban{
		Strikes: 1, Durations: []time.Duration{time.Hour, 2 * time.Hour}, Forget: 24 * time.Hour,
	})
	mustAllow(t, l, rule, "account")
	if res := mustAllow(t, l, rule, "account"); !res.Banned {
		t.Fatalf("want banned: %+v", res)
	}
	if err := l.Reset(ctx, rule, "account"); err != nil {
		t.Fatal(err)
	}
	if res := mustAllow(t, l, rule, "account"); !res.Allowed {
		t.Fatalf("after reset: %+v", res)
	}
	if res := mustAllow(t, l, rule, "account"); !res.Banned || res.RetryAfter != time.Hour {
		t.Fatalf("reset also forgets earlier bans, so this is the first step again: %+v", res)
	}
}

func TestRulesAndKeysAreIndependent(t *testing.T) {
	l, _ := newMiniLimiter(t)
	a := Sliding("rule-a", 1, time.Minute)
	b := Sliding("rule-b", 1, time.Minute)

	mustAllow(t, l, a, "ip-1")
	if res := mustAllow(t, l, a, "ip-1"); res.Allowed {
		t.Fatalf("rule-a exhausted: %+v", res)
	}
	if res := mustAllow(t, l, b, "ip-1"); !res.Allowed {
		t.Fatalf("rule-b has its own quota: %+v", res)
	}
	if res := mustAllow(t, l, a, "ip-2"); !res.Allowed {
		t.Fatalf("another key has its own quota: %+v", res)
	}
}

func TestAllowN(t *testing.T) {
	ctx := context.Background()
	l, _ := newMiniLimiter(t)
	rule := Sliding("batch", 5, time.Minute)

	res, err := l.AllowN(ctx, rule, "user", 3)
	if err != nil || !res.Allowed || res.Remaining != 2 {
		t.Fatalf("n=3: %+v %v", res, err)
	}
	res, err = l.AllowN(ctx, rule, "user", 3)
	if err != nil || res.Allowed || res.Remaining != 2 {
		t.Fatalf("n=3 over: %+v %v", res, err)
	}
	if _, err := l.AllowN(ctx, rule, "user", 6); err == nil {
		t.Fatal("n over capacity should error")
	}
	if _, err := l.AllowN(ctx, rule, "user", 0); err == nil {
		t.Fatal("n=0 should error")
	}
}

func TestErrors(t *testing.T) {
	ctx := context.Background()
	rule := Sliding("errors", 1, time.Minute)
	down := New(func() (redis.Cmdable, bool) { return nil, false })
	if _, err := down.Allow(ctx, rule, "ip"); err != ErrUnavailable {
		t.Fatalf("want ErrUnavailable, got %v", err)
	}
	l, _ := newMiniLimiter(t)
	if _, err := l.Allow(ctx, rule, ""); err != ErrEmptyKey {
		t.Fatalf("want ErrEmptyKey, got %v", err)
	}
}

func TestRedisKeysShareHashTag(t *testing.T) {
	hashTag := func(key string) string {
		start := strings.Index(key, "{")
		end := strings.Index(key[start+1:], "}")
		return key[start+1 : start+1+end]
	}
	for _, rule := range []Rule{Smooth("tag", 1, time.Second, 1), Sliding("tag", 1, time.Second), Fixed("tag", 1, time.Second)} {
		for _, key := range []string{"1.2.3.4", "a}b", "{x}", "/ser-api/auth|::1"} {
			keys := rule.redisKeys(key)
			for _, k := range keys[1:] {
				if hashTag(k) != hashTag(keys[0]) {
					t.Fatalf("%s %q: %q and %q land in different slots", rule, key, keys[0], k)
				}
			}
		}
	}
}

func TestRuleValidation(t *testing.T) {
	cases := map[string]func(){
		"empty name":    func() { Sliding(" ", 1, time.Second) },
		"brace in name": func() { Sliding("a{b", 1, time.Second) },
		"zero limit":    func() { Sliding("x", 0, time.Second) },
		"zero period":   func() { Smooth("x", 1, 0, 1) },
		"zero burst":    func() { Smooth("x", 1, time.Second, 0) },
		"zero strikes": func() {
			Sliding("x", 1, time.Second).WithBan(Ban{Durations: []time.Duration{time.Second}, Forget: time.Hour})
		},
		"strikes no within": func() {
			Sliding("x", 1, time.Second).WithBan(Ban{Strikes: 2, Durations: []time.Duration{time.Second}, Forget: time.Hour})
		},
		"no durations": func() { Sliding("x", 1, time.Second).WithBan(Ban{Strikes: 1, Forget: time.Hour}) },
		"tiny duration": func() {
			Sliding("x", 1, time.Second).WithBan(Ban{Strikes: 1, Durations: []time.Duration{0}, Forget: time.Hour})
		},
		"forget too short": func() {
			Sliding("x", 1, time.Second).WithBan(Ban{Strikes: 1, Durations: []time.Duration{time.Hour}, Forget: time.Minute})
		},
		"zero value use": func() { Middleware(Rule{}) },
	}
	for name, fn := range cases {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("want panic")
				}
			}()
			fn()
		})
	}
}

func newEngine(mws ...gin.HandlerFunc) *gin.Engine {
	r := gin.New()
	r.Use(mws...)
	ok := func(c *gin.Context) { c.String(http.StatusOK, "ok") }
	r.GET("/health", ok)
	r.GET("/a", ok)
	r.GET("/b", ok)
	r.GET("/group/c", ok)
	return r
}

func get(r *gin.Engine, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

func TestMiddlewareSharesQuotaAcrossRoutes(t *testing.T) {
	l, _ := newMiniLimiter(t)
	r := newEngine(Middleware(Sliding("global", 2, time.Minute), WithLimiter(l)))

	if w := get(r, "/a"); w.Code != http.StatusOK || w.Header().Get("X-RateLimit-Remaining") != "1" {
		t.Fatalf("/a: %d %v", w.Code, w.Header())
	}
	if w := get(r, "/b"); w.Code != http.StatusOK {
		t.Fatalf("/b: %d", w.Code)
	}
	w := get(r, "/a")
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("third request across routes: %d", w.Code)
	}
	if got := w.Header().Get("Retry-After"); got != "60" {
		t.Fatalf("Retry-After %q", got)
	}
	if !strings.Contains(w.Body.String(), "42900") {
		t.Fatalf("body %s", w.Body.String())
	}
}

func TestMiddlewareByRoute(t *testing.T) {
	l, _ := newMiniLimiter(t)
	r := newEngine(Middleware(Sliding("per-route", 1, time.Minute), WithLimiter(l), WithKeyFunc(ByRoute)))
	if get(r, "/a").Code != http.StatusOK || get(r, "/b").Code != http.StatusOK {
		t.Fatal("each route has its own quota")
	}
	if get(r, "/a").Code != http.StatusTooManyRequests {
		t.Fatal("/a exhausted")
	}
}

func TestMiddlewareSkipPaths(t *testing.T) {
	l, _ := newMiniLimiter(t)
	r := newEngine(Middleware(Sliding("skip", 1, time.Minute), WithLimiter(l), WithSkip(SkipPaths("/health"))))
	get(r, "/a")
	for i := 0; i < 3; i++ {
		if w := get(r, "/health"); w.Code != http.StatusOK {
			t.Fatalf("/health must not be limited: %d", w.Code)
		}
	}
}

func TestMiddlewareLayersCountSeparately(t *testing.T) {
	l, _ := newMiniLimiter(t)
	r := gin.New()
	r.Use(Middleware(Sliding("outer", 10, time.Minute), WithLimiter(l)))
	g := r.Group("/group", Middleware(Sliding("inner", 1, time.Minute), WithLimiter(l)))
	g.GET("/c", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	if get(r, "/group/c").Code != http.StatusOK {
		t.Fatal("first request")
	}
	if get(r, "/group/c").Code != http.StatusTooManyRequests {
		t.Fatal("inner layer exhausted")
	}
	res, err := l.Status(context.Background(), Sliding("outer", 10, time.Minute), "192.0.2.1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Remaining != 8 {
		t.Fatalf("outer counts each request once: %+v", res)
	}
}

func TestMiddlewareDegrade(t *testing.T) {
	down := New(func() (redis.Cmdable, bool) { return nil, false })

	open := newEngine(Middleware(Sliding("degrade", 1, time.Minute), WithLimiter(down)))
	if w := get(open, "/a"); w.Code != http.StatusOK || w.Body.String() != "ok" {
		t.Fatalf("fail-open: %d %s", w.Code, w.Body.String())
	}

	closed := newEngine(Middleware(Sliding("degrade", 1, time.Minute), WithLimiter(down), WithFailOpen(false)))
	if w := get(closed, "/a"); !strings.Contains(w.Body.String(), "50002") {
		t.Fatalf("fail-closed: %d %s", w.Code, w.Body.String())
	}
}

func TestMiddlewareOnLimitedAndBodyStatus(t *testing.T) {
	l, _ := newMiniLimiter(t)
	var gotKey string
	var gotRes Result
	r := newEngine(Middleware(Sliding("hook", 1, time.Minute), WithLimiter(l), WithUseRealStatus(false),
		WithOnLimited(func(c *gin.Context, key string, res Result) { gotKey, gotRes = key, res })))
	get(r, "/a")
	w := get(r, "/a")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "42900") {
		t.Fatalf("UseRealStatus=false: %d %s", w.Code, w.Body.String())
	}
	if gotKey != "192.0.2.1" || gotRes.Allowed {
		t.Fatalf("hook: %q %+v", gotKey, gotRes)
	}
}

// 设置 RATELIMIT_TEST_REDIS_ADDR（例如 127.0.0.1:6379）时，用真实 Redis 再跑一遍脚本。
// key 带随机后缀，结束时 Reset，不碰别的数据。
func TestRealRedis(t *testing.T) {
	addr := os.Getenv("RATELIMIT_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("RATELIMIT_TEST_REDIS_ADDR 未设置")
	}
	client := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = client.Close() })
	l := New(func() (redis.Cmdable, bool) { return client, true })
	ctx := context.Background()
	key := "real-" + time.Now().Format("150405.000000")

	sliding := Sliding("real-sliding", 2, 300*time.Millisecond)
	smooth := Smooth("real-smooth", 2, 300*time.Millisecond, 2)
	fixed := Fixed("real-fixed", 2, 300*time.Millisecond)
	banned := Sliding("real-ban", 1, 200*time.Millisecond).WithBan(banOnce(400 * time.Millisecond))
	t.Cleanup(func() {
		for _, rule := range []Rule{sliding, smooth, fixed, banned} {
			_ = l.Reset(ctx, rule, key)
		}
	})

	for _, rule := range []Rule{sliding, smooth, fixed} {
		mustAllow(t, l, rule, key)
		mustAllow(t, l, rule, key)
		res := mustAllow(t, l, rule, key)
		if res.Allowed || res.RetryAfter <= 0 || res.RetryAfter > 300*time.Millisecond {
			t.Fatalf("%s over limit: %+v", rule, res)
		}
		time.Sleep(res.RetryAfter + 20*time.Millisecond)
		if res := mustAllow(t, l, rule, key); !res.Allowed {
			t.Fatalf("%s after RetryAfter: %+v", rule, res)
		}
	}

	mustAllow(t, l, banned, key)
	if res := mustAllow(t, l, banned, key); !res.Banned {
		t.Fatalf("ban: %+v", res)
	}
	time.Sleep(250 * time.Millisecond)
	if res := mustAllow(t, l, banned, key); !res.Banned {
		t.Fatalf("window has cleared but the ban still holds: %+v", res)
	}
	time.Sleep(200 * time.Millisecond)
	if res := mustAllow(t, l, banned, key); !res.Allowed {
		t.Fatalf("ban expired: %+v", res)
	}
}
