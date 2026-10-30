package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/boloc/go-frame-server/v2/pkg/errs"
	"github.com/boloc/go-frame-server/v2/pkg/frame/webx"
	"github.com/boloc/go-frame-server/v2/pkg/logger"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func listOrders() error { return errs.Database(errors.New("Error 1054")) }

func TestAccessLogOneLinePerRequest(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	t.Cleanup(logger.Replace(zap.New(core)))

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(AccessLog())
	r.GET("/orders", func(c *gin.Context) {
		ctx := c.Request.Context()
		logger.AddToScope(ctx, logger.String("order_id", "A1"))
		logger.AddToScope(ctx, logger.String("failed_sql", "SELECT 1"))
		webx.Fail(c, listOrders())
	})
	r.GET("/ok", func(c *gin.Context) {
		logger.AddToScope(c.Request.Context(), logger.String("order_id", "B2"))
		webx.Success(c, nil)
	})

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/orders", nil))
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/ok", nil))

	all := logs.All()
	if len(all) != 2 || all[0].Message != "http: access" || all[1].Message != "http: access" {
		t.Fatalf("want one http: access per request, got %+v", all)
	}
	failed, ok := all[0], all[1]
	if failed.Level != zapcore.ErrorLevel || ok.Level != zapcore.InfoLevel {
		t.Fatalf("levels = %v, %v; want error, info", failed.Level, ok.Level)
	}
	fields := failed.ContextMap()
	want := map[string]any{
		"biz_code":    int64(errs.CodeDatabase),
		"error":       "[50001] data processing failed: Error 1054",
		"origin_func": "middleware.listOrders",
		"failed_sql":  "SELECT 1",
		"order_id":    "A1",
	}
	for k, v := range want {
		if fields[k] != v {
			t.Errorf("field %s = %v, want %v", k, fields[k], v)
		}
	}
	if ok.ContextMap()["order_id"] != "B2" {
		t.Errorf("success line should carry scope fields too, got %v", ok.ContextMap())
	}
}

func TestAccessLogLevel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		status  int
		bizCode errs.Code
		hasBiz  bool
		slow    bool
		want    zapcore.Level
	}{
		{name: "未写业务码的 204 预检是 Info", status: http.StatusNoContent, bizCode: -1, want: zapcore.InfoLevel},
		{name: "成功且业务码 0 是 Info", status: http.StatusOK, hasBiz: true, want: zapcore.InfoLevel},
		{name: "HTTP 4xx 是 Warn", status: http.StatusNotFound, bizCode: -1, want: zapcore.WarnLevel},
		{name: "业务码 4xxxx 是 Warn", status: http.StatusOK, bizCode: errs.CodeNotFound, hasBiz: true, want: zapcore.WarnLevel},
		{name: "慢请求是 Warn", status: http.StatusOK, hasBiz: true, slow: true, want: zapcore.WarnLevel},
		{name: "没走 webx 的 HTTP 5xx 只到 Warn（panic 恢复已打 ERROR）", status: http.StatusInternalServerError, bizCode: -1, want: zapcore.WarnLevel},
		{name: "业务码 5xxxx 是 Error", status: http.StatusOK, bizCode: errs.CodeDatabase, hasBiz: true, want: zapcore.ErrorLevel},
		{name: "FailWithStatus 的 HTTP 5xx 是 Error", status: http.StatusInternalServerError, bizCode: errs.CodeInternal, hasBiz: true, want: zapcore.ErrorLevel},
		{name: "FailWithStatus 的 HTTP 4xx 是 Warn", status: http.StatusNotFound, bizCode: errs.CodeNotFound, hasBiz: true, want: zapcore.WarnLevel},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := accessLogLevel(tt.status, tt.bizCode, tt.hasBiz, tt.slow)
			if got != tt.want {
				t.Fatalf("accessLogLevel() = %v, want %v", got, tt.want)
			}
		})
	}
}
