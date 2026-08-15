package monitor

import (
	"context"
	"testing"
	"time"
)

// TestMetricsComponentStopActuallyStopsBackgroundGoroutine 验证 Stop 后后台采集协程会退出。
func TestMetricsComponentStopActuallyStopsBackgroundGoroutine(t *testing.T) {
	m := NewMetricsComponent(WithCollectInterval(10 * time.Millisecond))

	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := m.Stop(stopCtx); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	select {
	case <-m.done:
		// 后台协程已经退出，符合预期。
	default:
		t.Fatal("Stop() 返回之后，后台采集协程应该已经退出")
	}
}

func TestMetricsComponentStopWithoutStartIsNoop(t *testing.T) {
	m := NewMetricsComponent()
	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() without Start() should be a no-op, got error = %v", err)
	}
}
