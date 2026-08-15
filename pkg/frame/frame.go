// Package frame 提供组件生命周期：装配、启停钩子、信号优雅关闭与热重启。
// 配置由 main 加载后交给 bootstrap，Frame 本身不持有业务配置。
package frame

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/boloc/go-frame-server/pkg/alert"
	"github.com/boloc/go-frame-server/pkg/logger"
	"go.uber.org/zap"
)

// Component 跟随进程生命周期启动/停止的资源。
type Component interface {
	// Start 启动组件。返回 error 会导致 Frame.Start 中止并回滚已启动的组件。
	Start(ctx context.Context) error
	// Stop 停止组件。即使返回 error，Frame 也会继续停止其它组件（尽力关闭，不因单个组件失败而卡住整体退出）。
	Stop(ctx context.Context) error
}

// Hook 定义钩子函数类型，用于 AfterStart / BeforeStop。
type Hook func(ctx context.Context) error

// FrameConfig 框架自身的行为配置（不含任何业务配置）。
type FrameConfig struct {
	// ShutdownTimeout 优雅关闭超时。超时后仍会带着已取消的 ctx 继续 Stop 剩余组件。
	ShutdownTimeout time.Duration
	// EnableRestartSignal 是否响应 SIGHUP 热重启（重启组件但不退出进程），默认开启。
	EnableRestartSignal bool
}

// Option 定义框架选项函数类型。
type Option func(*Frame)

// WithShutdownTimeout 设置优雅关闭超时时间。
func WithShutdownTimeout(timeout time.Duration) Option {
	return func(f *Frame) {
		f.config.ShutdownTimeout = timeout
	}
}

// WithRestartSignal 控制是否响应 SIGHUP 触发热重启（见 Frame.Restart）。
func WithRestartSignal(enabled bool) Option {
	return func(f *Frame) {
		f.config.EnableRestartSignal = enabled
	}
}

// runState 用来在并发场景下判断 Frame 当前处于什么阶段，避免重复 Start/对未 Start 的实例调用 Stop。
type runState int32

const (
	stateIdle runState = iota
	stateRunning
	stateStopped
)

// Frame 框架核心结构。不含任何 package-level 全局状态，可以创建多个独立实例。
type Frame struct {
	mu         sync.RWMutex
	components []Component
	config     *FrameConfig
	state      runState

	afterStartHooks []Hook
	beforeStopHooks []Hook

	// singletons 记录 RegisterSingleton 已经用过的 key，见该方法的文档。
	singletons map[string]bool
}

// New 创建新的框架实例。
func New(opts ...Option) *Frame {
	f := &Frame{
		components: make([]Component, 0),
		config: &FrameConfig{
			ShutdownTimeout:     30 * time.Second,
			EnableRestartSignal: true,
		},
		afterStartHooks: make([]Hook, 0),
		beforeStopHooks: make([]Hook, 0),
		singletons:      make(map[string]bool),
	}

	for _, opt := range opts {
		opt(f)
	}

	return f
}

// AfterStart 注册启动后的钩子函数，按注册顺序依次执行；任意一个返回 error 会中止 Run。
func (f *Frame) AfterStart(hook Hook) *Frame {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.afterStartHooks = append(f.afterStartHooks, hook)
	return f
}

// BeforeStop 注册停止前的钩子函数，按注册顺序依次执行；某个钩子失败只记录日志，不阻止后续钩子和 Stop 执行。
func (f *Frame) BeforeStop(hook Hook) *Frame {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.beforeStopHooks = append(f.beforeStopHooks, hook)
	return f
}

// RegisterComponent 注册组件，按注册顺序启动、反序停止。同一类型可注册多次。
// 进程内只应有一个的组件请用 RegisterSingleton。
func (f *Frame) RegisterComponent(component Component) *Frame {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.components = append(f.components, component)
	return f
}

// RegisterSingleton 注册进程内单例组件，key 重复会 panic。
func (f *Frame) RegisterSingleton(key string, component Component) *Frame {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.singletons[key] {
		panic("frame: singleton component with key " + key + " is already registered, " +
			"call RegisterSingleton with this key only once per Frame")
	}
	f.singletons[key] = true
	f.components = append(f.components, component)
	return f
}

// Run 阻塞运行：Start → AfterStart 钩子 → 等待信号 → BeforeStop 钩子 → Stop。
// 等价于 RunContext(context.Background())。
func (f *Frame) Run() error {
	return f.RunContext(context.Background())
}

// RunContext 同 Run，使用调用方传入的根 context。
// SIGINT/SIGTERM 优雅退出；SIGHUP 默认触发 Restart，可用 WithRestartSignal(false) 关闭。
func (f *Frame) RunContext(ctx context.Context) error {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// 启动所有组件
	if err := f.Start(runCtx); err != nil {
		return err
	}
	f.logInfo("frame started successfully")

	// 执行 AfterStart 钩子
	if err := f.runHooks(runCtx, f.snapshotHooks(true)); err != nil {
		f.logError("after start hook failed", err)
		_ = f.Stop(runCtx)
		return err
	}

	// 监听信号
	signals := []os.Signal{syscall.SIGINT, syscall.SIGTERM}
	if f.config.EnableRestartSignal {
		signals = append(signals, syscall.SIGHUP)
	}
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, signals...)
	defer signal.Stop(sigChan)

	// 等待信号
	for {
		select {
		case <-ctx.Done():
			// 上下文取消，执行 shutdown
			return f.shutdown(runCtx)
		case sig := <-sigChan:
			if sig == syscall.SIGHUP {
				f.logInfo("received SIGHUP, restarting components")
				if err := f.Restart(runCtx); err != nil {
					f.logError("restart failed", err)
					return err
				}
				f.logInfo("restart completed, still serving")
				continue
			}
			f.logInfo("received signal " + sig.String() + ", shutting down")
			return f.shutdown(runCtx)
		}
	}
}

// shutdown 执行一次完整的优雅退出：BeforeStop 钩子 → Stop 所有组件。
func (f *Frame) shutdown(ctx context.Context) error {
	if err := f.runHooks(ctx, f.snapshotHooks(false)); err != nil {
		f.logError("before stop hook failed", err)
	}

	if err := f.Stop(ctx); err != nil {
		f.logError("error during framework shutdown", err)
		return err
	}

	f.logInfo("frame stopped gracefully")
	return nil
}

// Start 按注册顺序启动所有组件；某个组件启动失败时，回滚（反序 Stop）已启动的组件并返回错误。
func (f *Frame) Start(ctx context.Context) error {
	f.mu.Lock()
	if f.state == stateRunning {
		f.mu.Unlock()
		return nil
	}
	components := f.components
	f.mu.Unlock()

	for i, component := range components {
		if err := component.Start(ctx); err != nil {
			for j := i - 1; j >= 0; j-- {
				if stopErr := components[j].Stop(ctx); stopErr != nil {
					f.logError(fmt.Sprintf("error stopping component %d during startup rollback", j), stopErr)
				}
			}
			return fmt.Errorf("failed to start component %d: %w", i, err)
		}
	}

	f.mu.Lock()
	f.state = stateRunning
	f.mu.Unlock()
	return nil
}

// Stop 按注册的反序停止所有组件，尽力执行完所有组件的 Stop（单个组件失败不影响其它组件），
// 最终返回遇到的最后一个错误（如果有）。
func (f *Frame) Stop(ctx context.Context) error {
	f.mu.Lock()
	if f.state == stateStopped {
		f.mu.Unlock()
		return nil
	}
	components := f.components
	f.mu.Unlock()

	shutdownCtx, cancel := context.WithTimeout(ctx, f.config.ShutdownTimeout)
	defer cancel()

	var lastErr error
	for i := len(components) - 1; i >= 0; i-- {
		if err := components[i].Stop(shutdownCtx); err != nil {
			lastErr = err
			f.logError(fmt.Sprintf("error stopping component %d", i), err)
		}
	}

	f.mu.Lock()
	f.state = stateStopped
	f.mu.Unlock()
	return lastErr
}

// Restart 先 Stop 再 Start，不重新执行 AfterStart/BeforeStop。
func (f *Frame) Restart(ctx context.Context) error {
	if err := f.Stop(ctx); err != nil {
		f.logError("restart: stop phase failed, continuing to start", err)
	}
	return f.Start(ctx)
}

func (f *Frame) snapshotHooks(afterStart bool) []Hook {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if afterStart {
		out := make([]Hook, len(f.afterStartHooks))
		copy(out, f.afterStartHooks)
		return out
	}
	out := make([]Hook, len(f.beforeStopHooks))
	copy(out, f.beforeStopHooks)
	return out
}

func (f *Frame) runHooks(ctx context.Context, hooks []Hook) error {
	for _, hook := range hooks {
		if err := hook(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (f *Frame) logInfo(msg string) {
	logger.Info(msg)
}

func (f *Frame) logError(msg string, err error) {
	logger.Error(msg, zap.Error(err))
	alert.Notify(context.Background(), alert.Event{Scope: "frame", Message: msg, Err: err})
}
