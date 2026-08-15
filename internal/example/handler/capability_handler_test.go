package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/boloc/go-frame-server/pkg/errs"
	"github.com/boloc/go-frame-server/pkg/frame/webx"
	"github.com/gin-gonic/gin"
)

func newCapabilityRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.GET("/test/success", CapabilitySuccess)
	r.GET("/test/not-found", CapabilityNotFound)
	r.GET("/test/not-found-with-status", CapabilityNotFoundWithRealStatus)
	r.GET("/test/database-error", CapabilityDatabaseError)
	r.GET("/test/business-error", CapabilityBusinessError)
	r.GET("/test/validation-error", CapabilityValidationError)
	r.GET("/test/panic", CapabilityPanic)
	r.GET("/test/error-source", CapabilityErrorSource)
	return r
}

func getJSON(t *testing.T, r *gin.Engine, path string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w, decodeBody(t, w)
}

func TestCapabilityHandler_FailDefaultsToHTTP200(t *testing.T) {
	// webx.Fail 默认永远 200，业务码放在 body.code 里——这是大部分接口该用的写法。
	r := newCapabilityRouter()

	w, body := getJSON(t, r, "/test/not-found")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (webx.Fail default), body=%s", w.Code, w.Body.String())
	}
	if body["code"].(float64) != 40400 {
		t.Fatalf("code = %v, want 40400", body["code"])
	}

	w, body = getJSON(t, r, "/test/business-error")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (webx.Fail default), body=%s", w.Code, w.Body.String())
	}
	if body["code"].(float64) != 10001 {
		t.Fatalf("code = %v, want 10001", body["code"])
	}
}

func TestCapabilityHandler_FailWithStatusVariesHTTPStatus(t *testing.T) {
	// 需要真实 HTTP 状态码语义的接口显式用 webx.FailWithStatus，是接口自己的选择，
	// 不依赖任何全局开关，同一个进程里可以和 webx.Fail 的接口共存。
	r := newCapabilityRouter()

	w, body := getJSON(t, r, "/test/not-found-with-status")
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (webx.FailWithStatus), body=%s", w.Code, w.Body.String())
	}
	if body["code"].(float64) != 40400 {
		t.Fatalf("code = %v, want 40400", body["code"])
	}
}

func TestCapabilityHandler_OnFailHook(t *testing.T) {
	var gotRoute string
	var gotCode errs.Code
	webx.SetOnFail(func(route string, err *errs.Error) {
		gotRoute = route
		gotCode = err.Code
	})
	defer webx.SetOnFail(nil)

	r := newCapabilityRouter()
	w, body := getJSON(t, r, "/test/database-error")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (webx.Fail default), body=%s", w.Code, w.Body.String())
	}
	if body["code"].(float64) != 50001 { // errs.CodeDatabase
		t.Fatalf("code = %v, want 50001", body["code"])
	}
	if gotRoute != "/test/database-error" {
		t.Fatalf("OnFail route = %q, want /test/database-error", gotRoute)
	}
	if gotCode != errs.CodeDatabase {
		t.Fatalf("OnFail code = %v, want %v", gotCode, errs.CodeDatabase)
	}
}

func TestCapabilityHandler_ValidationErrorIncludesFields(t *testing.T) {
	r := newCapabilityRouter()
	w, body := getJSON(t, r, "/test/validation-error") // 不传 id，触发 validate:"required"

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (webx.Fail default), body=%s", w.Code, w.Body.String())
	}
	fields, ok := body["fields"].(map[string]any)
	if !ok || len(fields) == 0 {
		t.Fatalf("expected non-empty fields in response, got body=%s", w.Body.String())
	}
}

// TestCapabilityHandler_ErrorSourceIsAccurate 验证 Caller 指向真正构造错误的那一行。
func TestCapabilityHandler_ErrorSourceIsAccurate(t *testing.T) {
	r := newCapabilityRouter()
	w, body := getJSON(t, r, "/test/error-source")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", w.Code, w.Body.String())
	}
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object in response, got body=%s", w.Body.String())
	}
	caller, _ := data["caller"].(string)
	if !strings.Contains(caller, "capability_handler.go") {
		t.Fatalf("caller = %q，应该指向 capability_handler.go（simulateQueryFailure 所在文件）", caller)
	}
}

func TestCapabilityHandler_PanicIsRecoveredByGin(t *testing.T) {
	r := newCapabilityRouter()
	req := httptest.NewRequest(http.MethodGet, "/test/panic", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (gin.Recovery default), body=%s", w.Code, w.Body.String())
	}
}
