package components

import (
	"context"
	"errors"
	"fmt"
	"time"

	flog "github.com/boloc/go-frame-server/pkg/logger"

	"go.uber.org/zap"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// defaultSlowThreshold 是 MySQLConfig/ClickHouseGORMConfig.SlowThreshold 留空时的慢查询阈值。
const defaultSlowThreshold = 200 * time.Millisecond

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
}

var _ gormlogger.Interface = (*gormZapLogger)(nil)

// newGormLogger 创建 zap 后端的 GORM logger。component 打进日志字段用于区分 mysql/clickhouse。
func newGormLogger(component string, level gormlogger.LogLevel, slowThreshold time.Duration) gormlogger.Interface {
	if slowThreshold <= 0 {
		slowThreshold = defaultSlowThreshold
	}
	return &gormZapLogger{component: component, level: level, slowThreshold: slowThreshold}
}

func (l *gormZapLogger) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	c := *l
	c.level = level
	return &c
}

func (l *gormZapLogger) Info(_ context.Context, msg string, args ...any) {
	if l.level >= gormlogger.Info {
		flog.Info("gorm: "+fmt.Sprintf(msg, args...), zap.String("component", l.component))
	}
}

func (l *gormZapLogger) Warn(_ context.Context, msg string, args ...any) {
	if l.level >= gormlogger.Warn {
		flog.Warn("gorm: "+fmt.Sprintf(msg, args...), zap.String("component", l.component))
	}
}

func (l *gormZapLogger) Error(_ context.Context, msg string, args ...any) {
	if l.level >= gormlogger.Error {
		flog.Error("gorm: "+fmt.Sprintf(msg, args...), zap.String("component", l.component))
	}
}

// Trace 每条 SQL 执行完都会被 GORM 回调一次；这里按 错误 > 慢查询 > 普通 的优先级决定打不打、打什么级别。
func (l *gormZapLogger) Trace(_ context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	if l.level <= gormlogger.Silent {
		return
	}

	elapsed := time.Since(begin)
	switch {
	case err != nil && l.level >= gormlogger.Error && !errors.Is(err, gorm.ErrRecordNotFound):
		// ErrRecordNotFound 是业务层的正常分支（"查不到"），不是数据库错误，不进 ERROR 日志。
		sql, rows := fc()
		flog.Error("gorm: sql error", l.fields(sql, rows, elapsed, zap.Error(err))...)
	case l.slowThreshold > 0 && elapsed > l.slowThreshold && l.level >= gormlogger.Warn:
		sql, rows := fc()
		flog.Warn("gorm: slow sql", l.fields(sql, rows, elapsed, zap.Duration("threshold", l.slowThreshold))...)
	case l.level >= gormlogger.Info:
		sql, rows := fc()
		flog.Info("gorm: sql", l.fields(sql, rows, elapsed)...)
	}
}

func (l *gormZapLogger) fields(sql string, rows int64, elapsed time.Duration, extra ...zap.Field) []zap.Field {
	fields := make([]zap.Field, 0, 4+len(extra))
	fields = append(fields,
		zap.String("component", l.component),
		zap.String("sql", sql),
		zap.Int64("rows", rows),
		zap.Duration("elapsed", elapsed),
	)
	return append(fields, extra...)
}
