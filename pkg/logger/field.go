package logger

import (
	"time"

	"go.uber.org/zap"
)

// Field 是一条结构化日志字段。业务代码用本包的构造函数拼字段，不要再 import zap。
type Field = zap.Field

func String(key, val string) Field { return zap.String(key, val) }

func ByteString(key string, val []byte) Field { return zap.ByteString(key, val) }

func Int(key string, val int) Field { return zap.Int(key, val) }

func Int64(key string, val int64) Field { return zap.Int64(key, val) }

func Uint(key string, val uint) Field { return zap.Uint(key, val) }

func Uint64(key string, val uint64) Field { return zap.Uint64(key, val) }

func Bool(key string, val bool) Field { return zap.Bool(key, val) }

func Duration(key string, val time.Duration) Field { return zap.Duration(key, val) }

func Time(key string, val time.Time) Field { return zap.Time(key, val) }

func Any(key string, val any) Field { return zap.Any(key, val) }

// Err 对应 zap.Error。不能叫 Error，那个名字已经是打 error 日志的函数。
func Err(err error) Field { return zap.Error(err) }
