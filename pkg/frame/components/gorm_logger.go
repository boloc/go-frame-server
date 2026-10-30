package components

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/boloc/go-frame-server/v2/pkg/frame/reqctx"
	flog "github.com/boloc/go-frame-server/v2/pkg/logger"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// defaultSlowThreshold 是 MySQLConfig/ClickHouseGORMConfig.SlowThreshold 留空时的慢查询阈值。
const defaultSlowThreshold = 500 * time.Millisecond

// gormZapLogger 把 GORM 的日志接到 pkg/logger（zap），替代 gorm.io/gorm/logger.Default。
//
// 不用 GORM 自带的 Default logger 的原因：
//   - 它直接 log.New(os.Stdout) 输出，带 ANSI 颜色码，绕过 pkg/logger 的文件/JSON 通道——
//     生产环境只采集应用日志文件时，SQL 错误和慢查询全部丢失；
//   - 它默认不忽略 ErrRecordNotFound，业务里每一次 First() 未命中都会打一条 ERROR，
//     把真正的数据库故障淹没在噪音里。
//
// 级别语义与 GORM 一致：Silent 什么都不打；Error 只打 SQL 错误；Warn 再加慢查询；
// Info 打每一条 SQL（只应在本地开发用）。
type gormZapLogger struct {
	component     string
	level         gormlogger.LogLevel
	slowThreshold time.Duration
	logParams     bool
}

var (
	_ gormlogger.Interface = (*gormZapLogger)(nil)
	_ gorm.ParamsFilter    = (*gormZapLogger)(nil)
)

// newGormLogger 创建 zap 后端的 GORM logger。component 打进日志字段用于区分 mysql/clickhouse。
// logParams 为 false 时日志里的 SQL 只留 ? 占位符，不带参数值。
func newGormLogger(component string, level gormlogger.LogLevel, slowThreshold time.Duration, logParams bool) gormlogger.Interface {
	if slowThreshold <= 0 {
		slowThreshold = defaultSlowThreshold
	}
	return &gormZapLogger{component: component, level: level, slowThreshold: slowThreshold, logParams: logParams}
}

// ParamsFilter 由 GORM 在拼日志用的 SQL 前调用；只影响日志，不影响真正执行的 SQL。
// 默认不带参数值：密码哈希、手机号、邮箱这类值不进日志。
func (l *gormZapLogger) ParamsFilter(_ context.Context, sql string, params ...any) (string, []any) {
	if l.logParams {
		return sql, params
	}
	return sql, nil
}

func (l *gormZapLogger) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	c := *l
	c.level = level
	return &c
}

func (l *gormZapLogger) Info(_ context.Context, msg string, args ...any) {
	if l.level >= gormlogger.Info {
		flog.Info("gorm: "+fmt.Sprintf(msg, args...), flog.String("component", l.component))
	}
}

func (l *gormZapLogger) Warn(_ context.Context, msg string, args ...any) {
	if l.level >= gormlogger.Warn {
		flog.Warn("gorm: "+fmt.Sprintf(msg, args...), flog.String("component", l.component))
	}
}

func (l *gormZapLogger) Error(_ context.Context, msg string, args ...any) {
	if l.level >= gormlogger.Error {
		flog.Error("gorm: "+fmt.Sprintf(msg, args...), flog.String("component", l.component))
	}
}

// Trace 每条 SQL 执行完都会被 GORM 回调一次；这里按 错误 > 慢查询 > 普通 的优先级决定打不打、打什么级别。
//
// ctx 有日志作用域（HTTP 请求、cron 任务）时，SQL 错误挂到作用域上，由入口结束时的那一条日志带出；
// 没有作用域时自己打 ERROR。
func (l *gormZapLogger) Trace(ctx context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	if l.level <= gormlogger.Silent {
		return
	}

	elapsed := time.Since(begin)
	switch {
	case err != nil && l.level >= gormlogger.Error && !ignoredGormTraceErr(err):
		sql, rows := fc()
		if flog.AddToScope(ctx,
			flog.String("failed_sql", sql),
			flog.Duration("failed_sql_elapsed", elapsed),
			flog.String("failed_sql_error", err.Error()),
		) {
			return
		}
		flog.Error("gorm: sql error", l.fields(ctx, sql, rows, elapsed, flog.Err(err))...)
	case l.slowThreshold > 0 && elapsed > l.slowThreshold && l.level >= gormlogger.Warn:
		sql, rows := fc()
		flog.Warn("gorm: slow sql", l.fields(ctx, sql, rows, elapsed, flog.Duration("threshold", l.slowThreshold))...)
	case l.level >= gormlogger.Info:
		sql, rows := fc()
		flog.Info("gorm: sql", l.fields(ctx, sql, rows, elapsed)...)
	}
}

// ignoredGormTraceErr 这些不是数据库故障：查不到是业务分支，ctx 取消/超时是关进程或
// 请求截止，GORM 把它们包装成 SQL 错误，这里不当 ERROR。
func ignoredGormTraceErr(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound) ||
		errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded)
}

// 顺序按控制台从左往右读：耗时、阈值或错误、行数、SQL，最后是 request_id（请求内才有）和组件名。
func (l *gormZapLogger) fields(ctx context.Context, sql string, rows int64, elapsed time.Duration, extra ...flog.Field) []flog.Field {
	fields := make([]flog.Field, 0, 6+len(extra))
	fields = append(fields, flog.Duration("elapsed", elapsed))
	fields = append(fields, extra...)
	fields = append(fields,
		flog.Int64("rows", rows),
		flog.String("sql", sql),
	)
	if ctx != nil {
		if id := reqctx.FromContext(ctx).RequestID; id != "" {
			fields = append(fields, flog.String("request_id", id))
		}
	}
	return append(fields, flog.String("component", l.component))
}
