package util

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"
)

// TestGetClientReturnsSameInstance 验证多次 GetClient 返回同一实例。
func TestGetClientReturnsSameInstance(t *testing.T) {
	c1 := GetClient()
	c2 := GetClient()
	if c1 != c2 {
		t.Fatalf("GetClient() 两次调用返回了不同的实例，期望是同一个单例")
	}
}

// TestGetClientConcurrentSafe 验证并发首次调用 GetClient 仍返回同一实例。
func TestGetClientConcurrentSafe(t *testing.T) {
	const n = 50
	results := make([]*resty.Client, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := range n {
		go func() {
			defer wg.Done()
			results[i] = GetClient()
		}()
	}
	wg.Wait()

	for i := 1; i < n; i++ {
		if results[i] != results[0] {
			t.Fatalf("并发调用 GetClient() 拿到了不同的实例（第 %d 个和第 0 个不一致）", i)
		}
	}
}

// TestGetNamedClientIsolatesConfigPerName 验证不同 name 的 client 互相隔离，同名复用首次实例。
func TestGetNamedClientIsolatesConfigPerName(t *testing.T) {
	fast := GetNamedClient("fast-svc", HTTPClientOptions{Timeout: 100 * time.Millisecond})
	slow := GetNamedClient("slow-svc", HTTPClientOptions{Timeout: 8 * time.Second})

	if fast == slow {
		t.Fatalf("两个不同 name 的 GetNamedClient 返回了同一个实例")
	}

	// 同名重复调用复用首次实例，后续 opts 不生效。
	fastAgain := GetNamedClient("fast-svc", HTTPClientOptions{Timeout: 5 * time.Second})
	if fastAgain != fast {
		t.Fatalf("同一个 name 重复调用 GetNamedClient 返回了不同的实例")
	}
}

// TestRetryConditionSkipsNonIdempotentMethodsByDefault 验证默认只重试幂等方法，POST 失败不重试。
func TestRetryConditionSkipsNonIdempotentMethodsByDefault(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	t.Run("POST 不重试", func(t *testing.T) {
		attempts = 0
		c := newRestyClient(HTTPClientOptions{RetryCount: 2, RetryWaitTime: time.Millisecond, RetryMaxWaitTime: time.Millisecond})
		_, _ = c.R().Post(srv.URL)
		if attempts != 1 {
			t.Fatalf("POST 请求失败后发生了重试，实际尝试次数 = %d, want 1", attempts)
		}
	})

	t.Run("GET 会重试", func(t *testing.T) {
		attempts = 0
		c := newRestyClient(HTTPClientOptions{RetryCount: 2, RetryWaitTime: time.Millisecond, RetryMaxWaitTime: time.Millisecond})
		_, _ = c.R().Get(srv.URL)
		if attempts != 3 { // 1 次首发 + 2 次重试
			t.Fatalf("GET 请求失败后的尝试次数 = %d, want 3", attempts)
		}
	})

	t.Run("显式放开后 POST 也会重试", func(t *testing.T) {
		attempts = 0
		c := newRestyClient(HTTPClientOptions{
			RetryCount: 2, RetryWaitTime: time.Millisecond, RetryMaxWaitTime: time.Millisecond,
			RetryAllowNonIdempotent: true,
		})
		_, _ = c.R().Post(srv.URL)
		if attempts != 3 {
			t.Fatalf("放开非幂等重试后 POST 的尝试次数 = %d, want 3", attempts)
		}
	})
}

// TestRetryCountZeroFallsBackToDefaultNotDisabled 验证 RetryCount 为 0 回落默认值，禁用需传 NoRetry。
func TestRetryCountZeroFallsBackToDefaultNotDisabled(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	t.Run("RetryCount 0 回落到默认值 2，会重试", func(t *testing.T) {
		attempts = 0
		c := newRestyClient(HTTPClientOptions{RetryCount: 0, RetryWaitTime: time.Millisecond, RetryMaxWaitTime: time.Millisecond})
		_, _ = c.R().Get(srv.URL)
		if attempts != 3 { // 1 次首发 + 2 次默认重试
			t.Fatalf("RetryCount:0 的尝试次数 = %d, want 3（应该回落到默认值 2，而不是被禁用）", attempts)
		}
	})

	t.Run("NoRetry 才是真正禁用重试", func(t *testing.T) {
		attempts = 0
		c := newRestyClient(HTTPClientOptions{RetryCount: NoRetry, RetryWaitTime: time.Millisecond, RetryMaxWaitTime: time.Millisecond})
		_, _ = c.R().Get(srv.URL)
		if attempts != 1 {
			t.Fatalf("RetryCount:NoRetry 的尝试次数 = %d, want 1（不应该重试）", attempts)
		}
	})
}

// TestDefaultClientActuallyWorks 验证默认 client 能发出真实请求。
func TestDefaultClientActuallyWorks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	resp, err := GetClient().R().Get(srv.URL)
	if err != nil {
		t.Fatalf("GetClient().R().Get(%q) 失败: %v", srv.URL, err)
	}
	if resp.StatusCode() != http.StatusOK {
		t.Fatalf("状态码 = %d, want %d", resp.StatusCode(), http.StatusOK)
	}
}
