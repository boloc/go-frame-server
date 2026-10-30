package webx

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/boloc/go-frame-server/v2/pkg/alert"
	"github.com/boloc/go-frame-server/v2/pkg/errs"
	"github.com/boloc/go-frame-server/v2/pkg/frame/reqctx"
	"github.com/boloc/go-frame-server/v2/pkg/logger"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

var errSchemaBehind = errors.New("Error 1054: Unknown column")

// 走真实路由，c.FullPath() 才有值。
func serveFail(t *testing.T, ctx context.Context, err error) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/questions", func(c *gin.Context) { Fail(c, err) })

	req := httptest.NewRequest(http.MethodGet, "/questions", nil).WithContext(ctx)
	r.ServeHTTP(httptest.NewRecorder(), req)
}

func listEnabled() error {
	return errs.Database(errSchemaBehind)
}

func scopeMap(ctx context.Context) map[string]any {
	enc := zapcore.NewMapObjectEncoder()
	for _, f := range logger.ScopeFields(ctx) {
		f.AddTo(enc)
	}
	return enc.Fields
}

func TestFailInScopeAddsErrorAndAlerts(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	t.Cleanup(logger.Replace(zap.New(core)))

	events := make(chan alert.Event, 1)
	alert.SetHook(func(_ context.Context, e alert.Event) { events <- e })
	t.Cleanup(func() { alert.SetHook(nil) })

	ctx := logger.NewScope(reqctx.NewContext(context.Background(), &reqctx.RequestContext{RequestID: "rid-42"}))
	serveFail(t, ctx, listEnabled())

	if logs.Len() != 0 {
		t.Fatalf("有作用域时 webx 不应自己打日志，got %+v", logs.All())
	}
	fields := scopeMap(ctx)
	if want := "[50001] data processing failed: " + errSchemaBehind.Error(); fields["error"] != want {
		t.Errorf("error = %v, want %q", fields["error"], want)
	}
	if fields["origin_func"] != "webx.listEnabled" {
		t.Errorf("origin_func = %v", fields["origin_func"])
	}
	if origin, _ := fields["origin"].(string); !strings.Contains(origin, "log_failure_test.go:") {
		t.Errorf("origin = %v, want log_failure_test.go:<line>", fields["origin"])
	}

	select {
	case e := <-events:
		if e.Fields["request_id"] != "rid-42" || e.Fields["origin_func"] != "webx.listEnabled" {
			t.Errorf("alert fields = %v", e.Fields)
		}
	case <-time.After(time.Second):
		t.Fatal("alert not notified")
	}
}

func TestFailWithoutScopeLogsItself(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	t.Cleanup(logger.Replace(zap.New(core)))

	ctx := reqctx.NewContext(context.Background(), &reqctx.RequestContext{RequestID: "rid-7"})
	serveFail(t, ctx, listEnabled())
	serveFail(t, context.Background(), errs.NotFound(""))

	entries := logs.FilterMessage("webx: request failed").All()
	if len(entries) != 2 || entries[0].Level != zapcore.ErrorLevel || entries[1].Level != zapcore.WarnLevel {
		t.Fatalf("want ERROR then WARN, got %+v", entries)
	}
	fields := entries[0].ContextMap()
	want := map[string]any{
		"request_id":  "rid-7",
		"route":       "/questions",
		"code":        int64(errs.CodeDatabase),
		"origin_func": "webx.listEnabled",
	}
	for k, v := range want {
		if fields[k] != v {
			t.Errorf("field %s = %v, want %v", k, fields[k], v)
		}
	}
	if _, ok := fields["caller"]; ok {
		t.Error("caller key collides with zap CallerKey")
	}
}

func TestFailKeepsOuterWrapText(t *testing.T) {
	ctx := logger.NewScope(context.Background())
	serveFail(t, ctx, fmt.Errorf("load questions for user 42: %w", listEnabled()))

	fields := scopeMap(ctx)
	if want := "load questions for user 42: [50001] data processing failed: " + errSchemaBehind.Error(); fields["error"] != want {
		t.Fatalf("error = %v, want %q", fields["error"], want)
	}
	if fields["origin_func"] != "webx.listEnabled" {
		t.Fatalf("origin_func = %v", fields["origin_func"])
	}
}

func TestFailTypedNilDoesNotPanic(t *testing.T) {
	ctx := logger.NewScope(context.Background())
	var typedNil *errs.Error
	serveFail(t, ctx, typedNil)

	if fields := scopeMap(ctx); fields["error"] == nil {
		t.Fatalf("typed-nil 应兜底成 50000 并记录，got %v", fields)
	}
}

func TestFailPlainErrorPointsAtHandler(t *testing.T) {
	ctx := logger.NewScope(context.Background())
	serveFail(t, ctx, errors.New("plain"))

	if want := "webx.serveFail.func1"; scopeMap(ctx)["origin_func"] != want {
		t.Fatalf("origin_func = %v, want handler %q", scopeMap(ctx)["origin_func"], want)
	}
}
