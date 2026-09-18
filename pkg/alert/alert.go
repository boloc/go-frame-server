// Package alert 提供全局失败通知出口。未注册 Hook 时 Notify 是 no-op。
package alert

import (
	"context"
	"sync/atomic"

	"github.com/boloc/go-frame-server/v2/pkg/logger"
	"go.uber.org/zap"
)

// Event 一次失败通知。字段可能为空，Hook 应对空值宽容。
type Event struct {
	Scope   string         // 报警范围，例如 "lifecycle"、"business"、"system"
	Name    string         // 报警名称，例如 "service_down"、"database_error"、"cache_miss"
	Message string         // 报警消息，例如 "服务下线"、"数据库错误"、"缓存丢失"
	Err     error          // 报警错误，例如 "service_down"、"database_error"、"cache_miss"
	Fields  map[string]any // 报警字段，例如 "service_name"、"database_name"、"cache_name"
}

type Hook func(ctx context.Context, e Event)

var hook atomic.Pointer[Hook]

// MaxInFlight 同时在跑的 Hook 上限。Notify 是异步的：如果没有这个上限，依赖故障时
// 每秒几百个失败请求都各起一个 goroutine 去调 Hook（通常是发 webhook，慢且会超时），
// goroutine 数和出站连接数会跟着错误率一起飙——本来是来报警的，结果把自己也拖垮。
// 超过上限的事件直接丢弃并计数（见 Dropped），首条和之后每 dropLogEvery 条打一条 Warn。
const MaxInFlight = 64

const dropLogEvery = 100

var (
	inFlight = make(chan struct{}, MaxInFlight)
	dropped  atomic.Uint64
)

// SetHook 注册或清空全局 Hook。建议在启动阶段调用一次。
func SetHook(h Hook) {
	if h == nil {
		hook.Store(nil)
		return
	}
	hook.Store(&h)
}

// Dropped 返回因为并发 Hook 数达到 MaxInFlight 而被丢弃的事件累计数，可以接进指标。
func Dropped() uint64 {
	return dropped.Load()
}

// Notify 异步触发通知；未注册 Hook 时为 no-op。并发 Hook 数达到 MaxInFlight 时丢弃本次事件。
func Notify(ctx context.Context, e Event) {
	h := hook.Load()
	if h == nil {
		return
	}

	select {
	case inFlight <- struct{}{}:
	default:
		n := dropped.Add(1)
		if n == 1 || n%dropLogEvery == 0 {
			logger.Warn("alert: too many hooks in flight, dropping event",
				zap.Uint64("dropped_total", n), zap.Int("max_in_flight", MaxInFlight),
				zap.String("scope", e.Scope), zap.String("name", e.Name))
		}
		return
	}

	notifyCtx := context.WithoutCancel(ctx)
	go func() {
		defer func() { <-inFlight }()
		defer func() {
			if r := recover(); r != nil {
				logger.Error("alert: hook panicked",
					zap.Any("panic", r), zap.String("scope", e.Scope), zap.String("name", e.Name))
			}
		}()
		(*h)(notifyCtx, e)
	}()
}
