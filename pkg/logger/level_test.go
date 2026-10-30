package logger_test

import (
	"testing"

	"github.com/boloc/go-frame-server/v2/pkg/logger"
	"go.uber.org/zap/zapcore"
)

func TestLevelsMatchZap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		got  logger.Level
		want zapcore.Level
		name string
	}{
		{logger.DebugLevel, zapcore.DebugLevel, "debug"},
		{logger.InfoLevel, zapcore.InfoLevel, "info"},
		{logger.WarnLevel, zapcore.WarnLevel, "warn"},
		{logger.ErrorLevel, zapcore.ErrorLevel, "error"},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %v, want zap %v", tt.name, tt.got, tt.want)
		}
		if s := logger.LevelToString(tt.want); s != tt.name {
			t.Errorf("LevelToString(%v) = %q, want %q", tt.want, s, tt.name)
		}
		if l, err := logger.ParseLevel(tt.name); err != nil || l != tt.want {
			t.Errorf("ParseLevel(%q) = %v, %v", tt.name, l, err)
		}
	}
}
