package logger

import (
	"errors"
	"time"

	"github.com/boloc/go-frame-server/v2/pkg/errs"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
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

// Err 输出 error 字段。错误链上有 *errs.Error 时再平铺 origin_func、origin（最初出错的位置），
// 不管错误被包了几层、在哪里打日志，都能直接看到源头。
// 不用 caller 作键：zap 的 CallerKey 已是 caller（打日志的位置），JSON 里会重名。
// 不能叫 Error，那个名字已经是打 error 日志的函数。
func Err(err error) Field {
	if err == nil {
		return zap.Skip()
	}
	var e *errs.Error
	if !errors.As(err, &e) || e == nil {
		return zap.Error(err)
	}
	return zap.Inline(errDetail{err: err, origin: e})
}

type errDetail struct {
	err    error
	origin *errs.Error
}

func (d errDetail) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("error", d.err.Error())
	if d.origin.Func != "" {
		enc.AddString("origin_func", d.origin.Func)
	}
	if d.origin.Caller != "" {
		enc.AddString("origin", d.origin.Caller)
	}
	return nil
}
