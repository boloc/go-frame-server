package logger

import (
	"context"
	"sync"
)

// scope 是一次操作（一个 HTTP 请求、一次 cron 任务）的日志作用域。下层把字段挂上来，
// 入口在结束时打的那一条日志统一带出，一次操作一条日志。
type scope struct {
	mu     sync.Mutex
	fields []Field
	index  map[string]int
	ended  bool
}

type scopeKey struct{}

// NewScope 在 ctx 上开一个日志作用域。由入口调用（AccessLog、cron 执行器），
// 入口结束时用 EndScope 取出字段拼进它打的那一条日志。
func NewScope(ctx context.Context) context.Context {
	return context.WithValue(ctx, scopeKey{}, &scope{index: map[string]int{}})
}

// AddToScope 把字段挂到 ctx 所在的作用域，同名字段后写覆盖。
// ctx 没有作用域、或作用域已被入口 EndScope（比如请求结束后还在跑的 goroutine）时返回 false，
// 调用方应自己把这些内容记成日志。
func AddToScope(ctx context.Context, fields ...Field) bool {
	s := scopeOf(ctx)
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ended {
		return false
	}
	for _, f := range fields {
		// logger.Err 这类平铺字段的 Key 为空，不参与按名覆盖。
		if f.Key == "" {
			s.fields = append(s.fields, f)
			continue
		}
		if i, ok := s.index[f.Key]; ok {
			s.fields[i] = f
			continue
		}
		s.index[f.Key] = len(s.fields)
		s.fields = append(s.fields, f)
	}
	return true
}

// EndScope 由开作用域的入口在打它那一条日志前调用：返回已挂上的字段，并关闭作用域。
// 之后的 AddToScope 返回 false，晚到的内容由调用方自己记日志，不会挂上去无人输出。
func EndScope(ctx context.Context) []Field {
	s := scopeOf(ctx)
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ended = true
	return append([]Field(nil), s.fields...)
}

// ScopeFields 返回 ctx 所在作用域里已挂上的字段，不关闭作用域；没有作用域时返回 nil。
func ScopeFields(ctx context.Context) []Field {
	s := scopeOf(ctx)
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Field(nil), s.fields...)
}

func scopeOf(ctx context.Context) *scope {
	if ctx == nil {
		return nil
	}
	s, _ := ctx.Value(scopeKey{}).(*scope)
	return s
}
