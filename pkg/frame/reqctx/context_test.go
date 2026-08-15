package reqctx

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestFromGinNeverPanicsWithoutMiddleware 验证未经过中间件时 FromGin 不 panic，且不返回 nil。
func TestFromGinNeverPanicsWithoutMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/no-middleware", nil)

	rc := FromGin(c) // 没有任何中间件往 context 里塞过 RequestContext

	if rc == nil {
		t.Fatal("FromGin should never return nil")
	}
	if rc.GinContext != c {
		t.Fatal("FromGin should still attach the current gin.Context")
	}
}

func TestFromContextReturnsEmptyNotNilWhenMissing(t *testing.T) {
	rc := FromContext(context.Background())
	if rc == nil {
		t.Fatal("FromContext should never return nil")
	}
}

func TestSetGetRoundTrip(t *testing.T) {
	rc := &RequestContext{}
	rc.Set("uid", 42)

	v, ok := rc.GetInt("uid")
	if !ok || v != 42 {
		t.Fatalf("GetInt() = (%v, %v), want (42, true)", v, ok)
	}

	if _, ok := rc.GetString("uid"); ok {
		t.Fatal("GetString() should fail for a value stored as int")
	}
}

func TestNewContextRoundTrip(t *testing.T) {
	rc := &RequestContext{RequestBody: []byte(`{"a":1}`)}
	ctx := NewContext(context.Background(), rc)

	got := FromContext(ctx)
	if got != rc {
		t.Fatal("FromContext should return the exact RequestContext stored by NewContext")
	}
}
