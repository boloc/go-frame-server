package middleware

import (
	"net/http"
	"testing"

	"github.com/boloc/go-frame-server/v2/pkg/errs"
	"go.uber.org/zap/zapcore"
)

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
		{name: "HTTP 5xx 是 Error", status: http.StatusInternalServerError, bizCode: -1, want: zapcore.ErrorLevel},
		{name: "业务码 5xxxx 是 Error", status: http.StatusOK, bizCode: errs.CodeInternal, hasBiz: true, want: zapcore.ErrorLevel},
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
