package frame

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"go.uber.org/zap"
)

// Component 定义组件接口
type Component interface {
	// Start 启动组件
	Start(ctx context.Context) error
	// Stop 停止组件
	Stop(ctx context.Context) error
}

// Hook 定义钩子函数类型
type Hook func(ctx context.Context) error

// FrameConfig 框架配置
type FrameConfig struct {
	ShutdownTimeout time.Duration
}

// Option 定义框架选项函数类型
type Option func(*Frame)

// WithShutdownTimeout 设置关闭超时时间
func WithShutdownTimeout(timeout time.Duration) Option {
	return func(f *Frame) {
		f.config.ShutdownTimeout = timeout
	}
}

// Frame 框架核心结构
type Frame struct {
	components []Component
	logger     *zap.Logger
	config     *FrameConfig
	mu         sync.RWMutex

	// 钩子函数
	afterStartHooks []Hook
	beforeStopHooks []Hook
}

// New 创建新的框架实例
func New(opts ...Option) *Frame {
	f := &Frame{
		components: make([]Component, 0), // 组件列表
		config: &FrameConfig{
			ShutdownTimeout: 30 * time.Second, // 默认30秒超时
		},
		afterStartHooks: make([]Hook, 0),
		beforeStopHooks: make([]Hook, 0),
	}

	for _, opt := range opts {
		opt(f)
	}

	return f
}

// AfterStart 注册启动后的钩子函数
func (f *Frame) AfterStart(hook Hook) *Frame {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.afterStartHooks = append(f.afterStartHooks, hook)
	return f
}

// BeforeStop 注册停止前的钩子函数
func (f *Frame) BeforeStop(hook Hook) *Frame {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.beforeStopHooks = append(f.beforeStopHooks, hook)
	return f
}

// RegisterComponent 注册组件
func (f *Frame) RegisterComponent(component Component) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.components = append(f.components, component)
}

// SetLogger 设置日志记录器
func (f *Frame) SetLogger(logger *zap.Logger) {
	f.logger = logger
}

// Run 运行框架并处理信号
func (f *Frame) Run() error {
	// 创建根上下文
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 启动框架
	if err := f.Start(ctx); err != nil {
		return err
	}

	if f.logger != nil {
		f.logger.Info("Framework started successfully")
	}

	// 执行启动后的钩子函数
	for _, hook := range f.afterStartHooks {
		if err := hook(ctx); err != nil {
			if f.logger != nil {
				f.logger.Error("Error executing after start hook", zap.Error(err))
			}
			return err
		}
	}

	// 设置信号处理
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 等待信号
	sig := <-sigChan
	fmt.Println("\n收到 Ctrl+C，正在退出...")
	if f.logger != nil {
		f.logger.Debug("接收到退出信号", zap.String("信号signal", sig.String()))
	}

	// 执行停止前的钩子函数
	for _, hook := range f.beforeStopHooks {
		if err := hook(ctx); err != nil {
			if f.logger != nil {
				f.logger.Error("Error executing before stop hook", zap.Error(err))
			}
			// 继续执行其他钩子，但记录错误
		}
	}

	// 优雅关闭
	if err := f.Stop(ctx); err != nil {
		if f.logger != nil {
			f.logger.Error("Error during framework shutdown", zap.Error(err))
		}
		return err
	}

	if f.logger != nil {
		f.logger.Info("Framework stopped gracefully")
	}

	return nil
}

// Start 启动框架
func (f *Frame) Start(ctx context.Context) error {
	f.mu.RLock()
	defer f.mu.RUnlock()

	// 启动所有组件
	for i, component := range f.components {
		if err := component.Start(ctx); err != nil {
			// 启动失败时，停止已启动的组件
			for j := i - 1; j >= 0; j-- {
				if stopErr := f.components[j].Stop(ctx); stopErr != nil {
					// 记录错误但继续关闭
					if f.logger != nil {
						f.logger.Error("Error stopping component during startup failure",
							zap.Int("component_index", j),
							zap.Error(stopErr),
						)
					}
				}
			}
			return fmt.Errorf("failed to start component %d: %v", i, err)
		}
	}
	return nil
}

// Stop 停止框架
func (f *Frame) Stop(ctx context.Context) error {
	f.mu.RLock()
	defer f.mu.RUnlock()

	// 创建带超时的上下文
	shutdownCtx, cancel := context.WithTimeout(ctx, f.config.ShutdownTimeout)
	defer cancel()

	// 按照注册的反序停止组件
	var lastErr error
	for i := len(f.components) - 1; i >= 0; i-- {
		if err := f.components[i].Stop(shutdownCtx); err != nil {
			lastErr = err
			if f.logger != nil {
				f.logger.Error("Error stopping component",
					zap.Int("component_index", i),
					zap.Error(err),
				)
			}
		}
	}
	return lastErr
}
