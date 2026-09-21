package refreshcache

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/redis/go-redis/v9"
)

func noRedis() (redis.Cmdable, bool) {
	return nil, false
}

func TestCollectorsIsRefreshcacheNotCron(t *testing.T) {
	c := New(Options[string]{Key: "site_setting"})
	cols := c.Collectors()
	if len(cols) != 1 {
		t.Fatalf("collectors=%d, want 1", len(cols))
	}
	if len(c.cron.Collectors()) != 0 {
		t.Fatal("internal cron should use WithoutMetrics")
	}

	descCh := make(chan *prometheus.Desc, 4)
	cols[0].Describe(descCh)
	got := (<-descCh).String()
	if !strings.Contains(got, "refreshcache_refresh_total") {
		t.Fatalf("desc=%s, want refreshcache_refresh_total", got)
	}
	if strings.Contains(got, "cron_task_") {
		t.Fatalf("cache collectors leaked cron metric: %s", got)
	}
}

func TestObserveRefreshFailureAndCanceled(t *testing.T) {
	c := New(Options[string]{
		Key:    "nav",
		Loader: func(context.Context) (string, error) { return "v", nil },
		Redis:  noRedis,
	})
	if err := c.refreshSourceToRedis(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := counterValue(t, c.refreshTotal, stageSourceToRedis, "skipped"); got != 1 {
		t.Fatalf("redis unavailable should count skipped, got %v", got)
	}
	if got := counterValue(t, c.refreshTotal, stageSourceToRedis, "failure"); got != 0 {
		t.Fatalf("redis unavailable must not count failure, got %v", got)
	}

	fail := New(Options[string]{
		Key:    "fail-cache",
		Loader: func(context.Context) (string, error) { return "", errors.New("db down") },
		Redis:  noRedis,
	})
	if err := fail.refreshSourceToRedis(context.Background()); err == nil {
		t.Fatal("expected loader error")
	}
	if got := counterValue(t, fail.refreshTotal, stageSourceToRedis, "failure"); got != 1 {
		t.Fatalf("loader failure count=%v", got)
	}

	canceled := New(Options[string]{
		Key: "canceled-cache",
		Loader: func(context.Context) (string, error) {
			return "", context.Canceled
		},
		Redis: noRedis,
	})
	if err := canceled.refreshSourceToRedis(context.Background()); err == nil {
		t.Fatal("expected canceled")
	}
	if got := counterValue(t, canceled.refreshTotal, stageSourceToRedis, "canceled"); got != 1 {
		t.Fatalf("canceled count=%v", got)
	}
}

func TestObserveRefreshRedisToMemoryFallback(t *testing.T) {
	c := New(Options[string]{
		Key:    "mem",
		Loader: func(context.Context) (string, error) { return "v", nil },
		Redis:  noRedis,
	})
	if err := c.refreshRedisToMemory(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := counterValue(t, c.refreshTotal, stageRedisToMemory, "success"); got != 1 {
		t.Fatalf("loader fallback success count=%v", got)
	}
}

func counterValue(t *testing.T, vec *prometheus.CounterVec, labelValues ...string) float64 {
	t.Helper()
	m, err := vec.GetMetricWithLabelValues(labelValues...)
	if err != nil {
		t.Fatal(err)
	}
	pb := &dto.Metric{}
	if err := m.Write(pb); err != nil {
		t.Fatal(err)
	}
	return pb.GetCounter().GetValue()
}
