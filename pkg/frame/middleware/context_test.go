package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/boloc/go-frame-server/pkg/frame/reqctx"
	"github.com/gin-gonic/gin"
)

func newTestEngine() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return gin.New()
}

func TestContextMiddlewareStoresQueryAndBody(t *testing.T) {
	engine := newTestEngine()
	engine.Use(ContextMiddleware())

	var gotBody string
	var gotQuery string
	engine.POST("/echo", func(c *gin.Context) {
		rc := reqctx.FromGin(c)
		gotBody = string(rc.RequestBody)
		gotQuery = string(rc.RequestQuery)
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodPost, "/echo?a=1", strings.NewReader(`{"x":1}`))
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if gotBody != `{"x":1}` {
		t.Fatalf("RequestBody = %q, want %q", gotBody, `{"x":1}`)
	}
	if gotQuery == "" {
		t.Fatal("RequestQuery should not be empty when the request has query params")
	}
}

// TestContextMiddlewarePopulatesRequestID 验证复用上游 X-Request-Id，并写回响应头。
func TestContextMiddlewarePopulatesRequestID(t *testing.T) {
	engine := newTestEngine()
	engine.Use(ContextMiddleware())

	var gotID string
	engine.GET("/echo", func(c *gin.Context) {
		gotID = reqctx.FromGin(c).RequestID
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/echo", nil)
	req.Header.Set(RequestIDHeader, "upstream-req-id")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if gotID != "upstream-req-id" {
		t.Fatalf("RequestID = %q, want %q（应该直接复用上游传入的值）", gotID, "upstream-req-id")
	}
	if got := w.Header().Get(RequestIDHeader); got != "upstream-req-id" {
		t.Fatalf("response header %s = %q, want %q", RequestIDHeader, got, "upstream-req-id")
	}
}

// TestContextMiddlewareGeneratesRequestIDWhenMissing 验证缺少上游 ID 时框架会自行生成。
func TestContextMiddlewareGeneratesRequestIDWhenMissing(t *testing.T) {
	engine := newTestEngine()
	engine.Use(ContextMiddleware())

	var gotID string
	engine.GET("/echo", func(c *gin.Context) {
		gotID = reqctx.FromGin(c).RequestID
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/echo", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if gotID == "" {
		t.Fatal("没有上游 X-Request-Id 时框架应该自己生成一个，不能是空字符串")
	}
	if got := w.Header().Get(RequestIDHeader); got != gotID {
		t.Fatalf("response header %s = %q，应该和 reqctx 里的 RequestID 一致", RequestIDHeader, got)
	}
}

// TestContextMiddlewarePopulatesClientIPAndUserAgent 验证会填充 ClientIP 和 UserAgent。
func TestContextMiddlewarePopulatesClientIPAndUserAgent(t *testing.T) {
	engine := newTestEngine()
	engine.Use(ContextMiddleware())

	var gotIP, gotUA string
	engine.GET("/echo", func(c *gin.Context) {
		rc := reqctx.FromGin(c)
		gotIP = rc.ClientIP
		gotUA = rc.UserAgent
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/echo", nil)
	req.Header.Set("User-Agent", "test-agent/1.0")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if gotIP == "" {
		t.Fatal("ClientIP 不应该为空")
	}
	if gotUA != "test-agent/1.0" {
		t.Fatalf("UserAgent = %q, want %q", gotUA, "test-agent/1.0")
	}
}

// TestContextMiddlewareRejectsBodyTooLarge 验证超限请求体返回 413，且不进入 handler。
func TestContextMiddlewareRejectsBodyTooLarge(t *testing.T) {
	engine := newTestEngine()
	engine.Use(MaxBodyBytes(8)) // 必须在 ContextMiddleware 之前注册才会生效
	engine.Use(ContextMiddleware())

	called := false
	engine.POST("/echo", func(c *gin.Context) {
		called = true
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader("this body is way over 8 bytes"))
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", w.Code)
	}
	if called {
		t.Fatal("handler 不应该被调用：请求体超限应该在 ContextMiddleware 就被拦下")
	}
}

func TestMaxBodyBytesAllowsBodyWithinLimit(t *testing.T) {
	engine := newTestEngine()
	engine.Use(MaxBodyBytes(1024))
	engine.Use(ContextMiddleware())
	engine.POST("/echo", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	req := httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader("small"))
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestRequestTimeoutCancelsContext(t *testing.T) {
	engine := newTestEngine()
	engine.Use(RequestTimeout(10 * time.Millisecond))

	var err error
	engine.GET("/slow", func(c *gin.Context) {
		<-c.Request.Context().Done()
		err = c.Request.Context().Err()
		c.String(http.StatusOK, "done")
	})

	req := httptest.NewRequest(http.MethodGet, "/slow", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if err == nil {
		t.Fatal("expected the request context to be cancelled after the timeout elapses")
	}
}
