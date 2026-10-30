package components

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/boloc/go-frame-server/v2/pkg/frame/reqctx"
	flog "github.com/boloc/go-frame-server/v2/pkg/logger"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// 替换的是包级 logger，用到它的测试不能 t.Parallel。
func observeLogs(t *testing.T) *observer.ObservedLogs {
	t.Helper()
	core, logs := observer.New(zapcore.DebugLevel)
	t.Cleanup(flog.Replace(zap.New(core)))
	return logs
}

func traceSQL(sql string) func() (string, int64) {
	return func() (string, int64) { return sql, 0 }
}

func scopeMap(ctx context.Context) map[string]any {
	enc := zapcore.NewMapObjectEncoder()
	for _, f := range flog.ScopeFields(ctx) {
		f.AddTo(enc)
	}
	return enc.Fields
}

func TestGormTraceInScopeAddsSQL(t *testing.T) {
	logs := observeLogs(t)
	l := newGormLogger("mysql", gormlogger.Warn, time.Second, false)

	ctx := flog.NewScope(context.Background())
	ctx, cancel := context.WithTimeout(ctx, time.Minute) // RequestTimeout 包在外层时也要取得到
	defer cancel()

	l.Trace(ctx, time.Now(), traceSQL("SELECT 1"), errors.New("first"))
	l.Trace(ctx, time.Now(), traceSQL("SELECT `missing` FROM t"), errors.New("Error 1054"))

	if logs.Len() != 0 {
		t.Fatalf("有作用域时不应自己打日志，got %d", logs.Len())
	}
	fields := scopeMap(ctx)
	if fields["failed_sql"] != "SELECT `missing` FROM t" || fields["failed_sql_error"] != "Error 1054" {
		t.Fatalf("scope fields = %v, want last failed SQL", fields)
	}
}

func TestGormParamsFilter(t *testing.T) {
	t.Parallel()

	hidden := newGormLogger("mysql", gormlogger.Warn, time.Second, false).(gorm.ParamsFilter)
	if sql, params := hidden.ParamsFilter(context.Background(), "INSERT INTO u (phone) VALUES (?)", "13800000000"); params != nil || sql != "INSERT INTO u (phone) VALUES (?)" {
		t.Fatalf("默认不应带参数值，got sql=%q params=%v", sql, params)
	}
	shown := newGormLogger("mysql", gormlogger.Warn, time.Second, true).(gorm.ParamsFilter)
	if _, params := shown.ParamsFilter(context.Background(), "SELECT ?", 1); len(params) != 1 {
		t.Fatalf("LogSQLParams=true 应保留参数，got %v", params)
	}
}

func TestGormTraceAfterScopeEndedLogsError(t *testing.T) {
	logs := observeLogs(t)
	l := newGormLogger("mysql", gormlogger.Error, time.Second, false)

	ctx := flog.NewScope(context.Background())
	flog.EndScope(ctx) // 请求已结束，异步 goroutine 还拿着这个 ctx
	l.Trace(ctx, time.Now(), traceSQL("SELECT 1"), errors.New("boom"))

	if n := logs.FilterMessage("gorm: sql error").FilterLevelExact(zapcore.ErrorLevel).Len(); n != 1 {
		t.Fatalf("作用域结束后 SQL 错误应自己打 ERROR，got %d", n)
	}
}

func TestGormTraceWithoutScopeLogsError(t *testing.T) {
	logs := observeLogs(t)
	l := newGormLogger("mysql", gormlogger.Error, time.Second, false)

	l.Trace(context.Background(), time.Now(), traceSQL("SELECT 1"), errors.New("boom"))

	entries := logs.FilterMessage("gorm: sql error").FilterLevelExact(zapcore.ErrorLevel).All()
	if len(entries) != 1 {
		t.Fatalf("ERROR sql error = %d, want 1", len(entries))
	}
	if got := entries[0].ContextMap()["sql"]; got != "SELECT 1" {
		t.Fatalf("sql field = %v", got)
	}
}

func TestGormTraceIgnoredErrors(t *testing.T) {
	logs := observeLogs(t)
	l := newGormLogger("mysql", gormlogger.Error, time.Second, false)

	ctx := flog.NewScope(context.Background())
	for _, err := range []error{gorm.ErrRecordNotFound, context.Canceled, context.DeadlineExceeded} {
		l.Trace(ctx, time.Now(), traceSQL("SELECT 1"), err)
		l.Trace(context.Background(), time.Now(), traceSQL("SELECT 1"), err)
	}
	if logs.Len() != 0 {
		t.Fatalf("ignored errors logged %d entries", logs.Len())
	}
	if n := len(flog.ScopeFields(ctx)); n != 0 {
		t.Fatalf("ignored errors should not be added to scope, got %d fields", n)
	}
}

func TestGormTraceSlowSQLStillWarns(t *testing.T) {
	logs := observeLogs(t)
	l := newGormLogger("mysql", gormlogger.Warn, time.Millisecond, false)

	ctx := flog.NewScope(reqctx.NewContext(context.Background(), &reqctx.RequestContext{RequestID: "rid-slow"}))
	l.Trace(ctx, time.Now().Add(-time.Second), traceSQL("SELECT SLEEP(1)"), nil)

	entries := logs.FilterMessage("gorm: slow sql").FilterLevelExact(zapcore.WarnLevel).All()
	if len(entries) != 1 {
		t.Fatalf("slow sql warn = %d, want 1", len(entries))
	}
	if got := entries[0].ContextMap()["request_id"]; got != "rid-slow" {
		t.Fatalf("request_id = %v, want rid-slow", got)
	}
}
