package refreshcache

import (
	"testing"
	"time"
)

func TestCapJitter(t *testing.T) {
	tests := []struct {
		jitter, interval, want time.Duration
	}{
		{0, time.Minute, 0},
		{-time.Second, time.Minute, 0},
		{time.Minute, 0, time.Minute},
		{150 * time.Second, 150 * time.Second, 150 * time.Second},
		{150 * time.Second, 45 * time.Second, 45 * time.Second},
		{10 * time.Second, 45 * time.Second, 10 * time.Second},
	}
	for _, tt := range tests {
		if got := capJitter(tt.jitter, tt.interval); got != tt.want {
			t.Fatalf("capJitter(%s, %s)=%s want %s", tt.jitter, tt.interval, got, tt.want)
		}
	}
}
