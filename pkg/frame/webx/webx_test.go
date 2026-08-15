package webx

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/boloc/go-frame-server/pkg/errs"
	"github.com/gin-gonic/gin"
)

func TestFailDefaultsToHTTP200(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, w := newTestContext()

	Fail(c, errs.NotFound("not found"))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestFailWithStatusVariesHTTPStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, w := newTestContext()

	FailWithStatus(c, errs.NotFound("not found"))

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

// TestConcurrentSetOnFailAndRespond 验证并发 SetOnFail 与 Fail 不会 data race。
func TestConcurrentSetOnFailAndRespond(t *testing.T) {
	gin.SetMode(gin.TestMode)
	defer SetOnFail(nil)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			SetOnFail(func(route string, err *errs.Error) {})
		}()
		go func() {
			defer wg.Done()
			c, _ := newTestContext()
			Fail(c, errs.Internal(errors.New("boom")))
		}()
	}
	wg.Wait()
}

// TestCallerHiddenFromResponseByDefault 验证默认响应体不包含 Caller。
func TestCallerHiddenFromResponseByDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, w := newTestContext()

	Fail(c, errs.Internal(errors.New("boom")))

	var resp Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Caller != "" {
		t.Fatalf("Caller = %q, 默认应该为空（IncludeCallerInResponse=false）", resp.Caller)
	}
}

// TestCallerIncludedInResponseWhenEnabled 验证打开开关后响应体包含 Caller。
func TestCallerIncludedInResponseWhenEnabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	IncludeCallerInResponse.Store(true)
	defer IncludeCallerInResponse.Store(false)

	c, w := newTestContext()
	Fail(c, errs.Internal(errors.New("boom"))) // 这一行应该出现在 resp.Caller 里

	var resp Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !strings.Contains(resp.Caller, "webx_test.go") {
		t.Fatalf("Caller = %q，应该指向本测试文件", resp.Caller)
	}
}

func newTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)
	return c, w
}
