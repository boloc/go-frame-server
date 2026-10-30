package logger_test

import (
	"context"
	"errors"
	"testing"

	"github.com/boloc/go-frame-server/v2/pkg/logger"
	"go.uber.org/zap/zapcore"
)

func TestScope(t *testing.T) {
	t.Parallel()

	if logger.AddToScope(context.Background(), logger.String("k", "v")) {
		t.Fatal("没有作用域时应返回 false")
	}
	if logger.ScopeFields(context.Background()) != nil {
		t.Fatal("没有作用域时应返回 nil")
	}

	ctx := logger.NewScope(context.Background())
	logger.AddToScope(ctx, logger.String("a", "1"), logger.String("b", "2"))
	logger.AddToScope(ctx, logger.String("a", "3"), logger.Err(errors.New("boom")))

	enc := zapcore.NewMapObjectEncoder()
	fields := logger.ScopeFields(ctx)
	for _, f := range fields {
		f.AddTo(enc)
	}
	if len(fields) != 3 || enc.Fields["a"] != "3" || enc.Fields["b"] != "2" || enc.Fields["error"] != "boom" {
		t.Fatalf("fields = %v (n=%d), want a 被覆盖、b 保留、error 追加", enc.Fields, len(fields))
	}
}

func TestEndScopeRejectsLateFields(t *testing.T) {
	t.Parallel()

	ctx := logger.NewScope(context.Background())
	logger.AddToScope(ctx, logger.String("a", "1"))
	if got := logger.EndScope(ctx); len(got) != 1 {
		t.Fatalf("EndScope() = %d fields, want 1", len(got))
	}
	if logger.AddToScope(ctx, logger.String("late", "x")) {
		t.Fatal("作用域结束后 AddToScope 应返回 false，让调用方自己记日志")
	}
	if n := len(logger.ScopeFields(ctx)); n != 1 {
		t.Fatalf("late field should not be added, got %d fields", n)
	}
}
