package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func newHTTPClientRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/test/http-client/retry-get", HTTPClientRetryOnIdempotent)
	r.GET("/test/http-client/no-retry-post", HTTPClientNoRetryOnPost)
	r.GET("/test/http-client/retry-post-allowed", HTTPClientRetryAllowNonIdempotent)
	r.GET("/test/http-client/named-timeout", HTTPClientNamedTimeout)
	return r
}

// TestHTTPClientRetryOnIdempotent 验证 GET（幂等方法）在下游前 2 次失败时会被自动
// 重试，第 3 次成功——这是 pkg/util.GetClient/GetNamedClient 默认重试策略要保证的行为。
func TestHTTPClientRetryOnIdempotent(t *testing.T) {
	r := newHTTPClientRouter()
	w := httptest.NewRequest(http.MethodGet, "/test/http-client/retry-get", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, w)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	body := decodeBody(t, rec)
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object in response, got body=%s", rec.Body.String())
	}
	if data["backend_hits"].(float64) != 3 {
		t.Fatalf("backend_hits = %v, want 3（1 次首发 + 2 次重试）", data["backend_hits"])
	}
	if data["status_code"].(float64) != http.StatusOK {
		t.Fatalf("status_code = %v, want 200", data["status_code"])
	}
}

// TestHTTPClientNoRetryOnPost 验证 POST（非幂等方法）默认不会重试：backend_hits 应该
// 停在 1，且响应把下游的 500 原样传导上来——这是"POST 重试等于可能重复提交"这条安全
// 默认值真正生效的证明，不是只停留在注释里。
func TestHTTPClientNoRetryOnPost(t *testing.T) {
	r := newHTTPClientRouter()
	req := httptest.NewRequest(http.MethodGet, "/test/http-client/no-retry-post", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	body := decodeBody(t, rec)
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object in response, got body=%s", rec.Body.String())
	}
	if data["backend_hits"].(float64) != 1 {
		t.Fatalf("backend_hits = %v, want 1（POST 默认不重试）", data["backend_hits"])
	}
	if data["status_code"].(float64) != http.StatusInternalServerError {
		t.Fatalf("status_code = %v, want 500（下游原样返回）", data["status_code"])
	}
}

// TestHTTPClientRetryAllowNonIdempotent 验证显式打开 RetryAllowNonIdempotent 之后，
// POST 也会被重试到成功——跟上一个测试正好构成对照组。
func TestHTTPClientRetryAllowNonIdempotent(t *testing.T) {
	r := newHTTPClientRouter()
	req := httptest.NewRequest(http.MethodGet, "/test/http-client/retry-post-allowed", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	body := decodeBody(t, rec)
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object in response, got body=%s", rec.Body.String())
	}
	if data["backend_hits"].(float64) != 3 {
		t.Fatalf("backend_hits = %v, want 3（显式放开非幂等重试之后）", data["backend_hits"])
	}
}

// TestHTTPClientNamedTimeout 验证按 name 隔离的超时配置真的互不影响：短超时的命名
// 客户端应该先失败，默认客户端应该等到下游返回成功。
func TestHTTPClientNamedTimeout(t *testing.T) {
	r := newHTTPClientRouter()
	req := httptest.NewRequest(http.MethodGet, "/test/http-client/named-timeout", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	body := decodeBody(t, rec)
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object in response, got body=%s", rec.Body.String())
	}
	if data["fast_client_timeout_error"].(string) == "" {
		t.Fatalf("期望短超时客户端报错，实际 fast_client_timeout_error 为空，body=%s", rec.Body.String())
	}
	if data["default_client_status"].(float64) != http.StatusOK {
		t.Fatalf("default_client_status = %v, want 200", data["default_client_status"])
	}
	if data["default_client_error"].(string) != "" {
		t.Fatalf("期望默认客户端正常返回，实际报错: %v", data["default_client_error"])
	}
}
