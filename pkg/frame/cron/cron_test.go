package cron

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestSleepJitterZeroReturnsImmediately(t *testing.T) {
	start := time.Now()
	if err := sleepJitter(context.Background(), 0); err != nil {
		t.Fatalf("sleepJitter(0) = %v", err)
	}
	if time.Since(start) > 50*time.Millisecond {
		t.Fatalf("zero jitter should not sleep, elapsed=%s", time.Since(start))
	}
}

func TestSleepJitterCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := sleepJitter(ctx, time.Second); err == nil {
		t.Fatal("expected canceled context error")
	}
}

func TestWrapDurationExcludesJitter(t *testing.T) {
	c := NewComponentWithOptions("test-jitter", []Option{WithDurationMetrics()}, Task{
		Name:     "quick",
		Schedule: "@every 1h",
		Jitter:   80 * time.Millisecond,
		Run: func(ctx context.Context) error {
			time.Sleep(10 * time.Millisecond)
			return nil
		},
	})

	start := time.Now()
	c.wrap(c.tasks[0])(context.Background())
	wall := time.Since(start)

	obs, err := c.duration.GetMetricWithLabelValues("quick")
	if err != nil {
		t.Fatal(err)
	}
	pb := &dto.Metric{}
	if err := obs.(prometheus.Metric).Write(pb); err != nil {
		t.Fatal(err)
	}
	h := pb.GetHistogram()
	if h.GetSampleCount() != 1 {
		t.Fatalf("sample count=%d", h.GetSampleCount())
	}
	avg := time.Duration(h.GetSampleSum() * float64(time.Second))
	if wall < 10*time.Millisecond {
		t.Fatalf("wall clock too small: %s", wall)
	}
	if avg > 40*time.Millisecond {
		t.Fatalf("duration metric included jitter: avg=%s wall=%s", avg, wall)
	}
}

func TestCollectorsOmitsDurationByDefault(t *testing.T) {
	plain := NewComponent("plain", Task{
		Name:     "n",
		Schedule: "@every 1h",
		Run:      func(context.Context) error { return nil },
	})
	if n := len(plain.Collectors()); n != 1 {
		t.Fatalf("default collectors=%d, want 1 (runs only)", n)
	}

	withDur := NewComponentWithOptions("with-dur", []Option{WithDurationMetrics()}, Task{
		Name:     "n",
		Schedule: "@every 1h",
		Run:      func(context.Context) error { return nil },
	})
	if n := len(withDur.Collectors()); n != 2 {
		t.Fatalf("duration collectors=%d, want 2", n)
	}
}

func TestWithoutMetricsCollectorsEmpty(t *testing.T) {
	c := NewComponentWithOptions("silent", []Option{WithoutMetrics()}, Task{
		Name:     "n",
		Schedule: "@every 1h",
		Run:      func(context.Context) error { return nil },
	})
	if n := len(c.Collectors()); n != 0 {
		t.Fatalf("WithoutMetrics collectors=%d, want 0", n)
	}
	if c.runsTotal != nil || c.duration != nil {
		t.Fatal("WithoutMetrics should leave runs/duration nil")
	}
}

func TestWithoutMetricsWrapDoesNotPanic(t *testing.T) {
	c := NewComponentWithOptions("silent-wrap", []Option{WithoutMetrics()}, Task{
		Name:     "n",
		Schedule: "@every 1h",
		Run:      func(context.Context) error { return nil },
	})
	c.wrap(c.tasks[0])(context.Background())
}

func TestWithoutMetricsWinsOverDuration(t *testing.T) {
	c := NewComponentWithOptions("off", []Option{WithoutMetrics(), WithDurationMetrics()}, Task{
		Name:     "n",
		Schedule: "@every 1h",
		Run:      func(context.Context) error { return nil },
	})
	if n := len(c.Collectors()); n != 0 {
		t.Fatalf("collectors=%d, want 0", n)
	}
}
