package logger_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/boloc/go-frame-server/v2/pkg/errs"
	"github.com/boloc/go-frame-server/v2/pkg/logger"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func repoFind() error { return errs.Database(errors.New("Error 1054")) }

func TestErrExpandsOrigin(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	t.Cleanup(logger.Replace(zap.New(core)))

	err := fmt.Errorf("cron task: %w", errs.Internal(repoFind()))
	logger.Error("cron: task failed", logger.Err(err))

	fields := logs.All()[0].ContextMap()
	if want := "logger_test.repoFind"; fields["origin_func"] != want {
		t.Fatalf("origin_func = %v, want %q", fields["origin_func"], want)
	}
	if origin, _ := fields["origin"].(string); !strings.Contains(origin, "field_test.go:") {
		t.Fatalf("origin = %v, want field_test.go:<line>", fields["origin"])
	}
	if want := "cron task: [50000] internal server error: [50001] data processing failed: Error 1054"; fields["error"] != want {
		t.Fatalf("error = %v, want %q", fields["error"], want)
	}
	if _, ok := fields["caller"]; ok {
		t.Fatal("caller key collides with zap CallerKey")
	}
}

func TestErrPlainAndNil(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	t.Cleanup(logger.Replace(zap.New(core)))

	logger.Error("plain", logger.Err(errors.New("boom")))
	logger.Error("nil", logger.Err(nil))

	plain := logs.All()[0].ContextMap()
	if plain["error"] != "boom" {
		t.Fatalf("error = %v", plain["error"])
	}
	if _, ok := plain["origin"]; ok {
		t.Fatal("plain error should not have origin")
	}
	if n := len(logs.All()[1].ContextMap()); n != 0 {
		t.Fatalf("nil error should add no field, got %d", n)
	}
}
