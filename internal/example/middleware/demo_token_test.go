package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func newTestEngine() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	group := r.Group("/test")
	group.Use(RequireDemoToken())
	group.GET("/ping", func(c *gin.Context) { c.String(http.StatusOK, "pong") })
	return r
}

func TestRequireDemoTokenRejectsRequestWithoutHeader(t *testing.T) {
	r := newTestEngine()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/test/ping", nil))

	// webx.Fail 默认 HTTP 200，业务码在 body.code。
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if w.Body.String() == "pong" {
		t.Fatal("缺 X-Demo-Token 时不应该放行到真正的 handler")
	}
}

func TestRequireDemoTokenAllowsRequestWithHeader(t *testing.T) {
	r := newTestEngine()
	req := httptest.NewRequest(http.MethodGet, "/test/ping", nil)
	req.Header.Set("X-Demo-Token", "anything")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK || w.Body.String() != "pong" {
		t.Fatalf("status=%d body=%q, want 200 and \"pong\"", w.Code, w.Body.String())
	}
}

// TestRequireDemoTokenDoesNotAffectOtherGroups 验证中间件只作用于挂载它的路由分组。
func TestRequireDemoTokenDoesNotAffectOtherGroups(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	testGroup := r.Group("/test")
	testGroup.Use(RequireDemoToken())
	testGroup.GET("/ping", func(c *gin.Context) { c.String(http.StatusOK, "pong") })
	r.GET("/other", func(c *gin.Context) { c.String(http.StatusOK, "ok") }) // 不在 /test 分组下

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/other", nil))

	if w.Code != http.StatusOK || w.Body.String() != "ok" {
		t.Fatalf("status=%d body=%q, want 200 and \"ok\"（/other 不应该受 /test 分组中间件影响）", w.Code, w.Body.String())
	}
}
