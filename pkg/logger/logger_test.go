package logger

import (
	"context"
	"testing"
)

// TestConvenienceFunctionsDoNotPanicBeforeStart 验证 Start 前调用 Debug/Info/Warn/Error 不会 panic，会退化到兜底 logger。
func TestConvenienceFunctionsDoNotPanicBeforeStart(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Debug/Info/Warn/Error 在 LoggerComponent.Start() 之前调用不应该 panic，got: %v", r)
		}
	}()

	Debug("debug before start")
	Info("info before start")
	Warn("warn before start")
	Error("error before start")
}

func TestGetLoggerNeverReturnsNil(t *testing.T) {
	l := NewLoggerComponent()
	if l.GetLogger() == nil {
		t.Fatal("GetLogger() 在 Start() 之前也不应该返回 nil")
	}
}

func TestGetLoggerReturnsRealLoggerAfterStart(t *testing.T) {
	// 只输出到 stdout，不落文件，避免测试在磁盘上留日志文件。
	l := NewLoggerComponent(WithLoggerStdout(true), WithLoggerIsFile(false))
	if err := l.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer l.Stop(context.Background())

	if l.GetLogger() == nil {
		t.Fatal("GetLogger() 在 Start() 之后不应该返回 nil")
	}
}

// TestLoggerComponentImplementsFrameComponent 验证 LoggerComponent 满足 Start/Stop 组件接口。
func TestLoggerComponentImplementsFrameComponent(t *testing.T) {
	var _ interface {
		Start(ctx context.Context) error
		Stop(ctx context.Context) error
	} = NewLoggerComponent()
}

func TestStopRevertsToFallbackLogger(t *testing.T) {
	l := NewLoggerComponent(WithLoggerStdout(true), WithLoggerIsFile(false))
	ctx := context.Background()
	if err := l.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if err := l.Stop(ctx); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	// Stop 之后包级便捷函数应该退化到兜底 logger，不panic、不写向已经"关闭"的 logger。
	Info("after stop")

	if l.GetLogger() == nil {
		t.Fatal("GetLogger() 在 Stop() 之后也不应该返回 nil")
	}
}

func TestStopWithoutStartIsNoop(t *testing.T) {
	l := NewLoggerComponent()
	if err := l.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() 在没 Start 过时应该是 no-op，got error: %v", err)
	}
}

func TestRestartRebuildsLogger(t *testing.T) {
	l := NewLoggerComponent(WithLoggerStdout(true), WithLoggerIsFile(false))
	ctx := context.Background()

	if err := l.Start(ctx); err != nil {
		t.Fatalf("first Start() error = %v", err)
	}
	if err := l.Stop(ctx); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if err := l.Start(ctx); err != nil {
		t.Fatalf("second Start() error = %v", err)
	}
	defer l.Stop(ctx)

	if l.GetLogger() == nil {
		t.Fatal("重新 Start() 之后 GetLogger() 不应该返回 nil")
	}
}
