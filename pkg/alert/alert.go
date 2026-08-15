// Package alert 提供全局失败通知出口。未注册 Hook 时 Notify 是 no-op。
package alert

import (
	"context"
	"sync/atomic"

	"github.com/boloc/go-frame-server/pkg/logger"
	"go.uber.org/zap"
)

// Event 一次失败通知。字段可能为空，Hook 应对空值宽容。
type Event struct {
	Scope   string
	Name    string
	Message string
	Err     error
	Fields  map[string]any
}

type Hook func(ctx context.Context, e Event)

var hook atomic.Pointer[Hook]

// SetHook 注册或清空全局 Hook。建议在启动阶段调用一次。
func SetHook(h Hook) {
	if h == nil {
		hook.Store(nil)
		return
	}
	hook.Store(&h)
}

// Notify 异步触发通知；未注册 Hook 时为 no-op。
func Notify(ctx context.Context, e Event) {
	h := hook.Load()
	if h == nil {
		return
	}

	notifyCtx := context.WithoutCancel(ctx)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("alert: hook panicked",
					zap.Any("panic", r), zap.String("scope", e.Scope), zap.String("name", e.Name))
			}
		}()
		(*h)(notifyCtx, e)
	}()
}
