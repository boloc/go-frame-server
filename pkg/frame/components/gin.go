package components

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"go-frame-server/pkg/constant"
	"go-frame-server/pkg/frame/middleware"

	"github.com/gin-gonic/gin"
)

// GinOption 定义Gin选项函数类型
type GinOption func(*GinComponent)

// GinComponent Gin组件
type GinComponent struct {
	engine *gin.Engine
	server *http.Server
	config *GinConfig
	// 路由注册函数
	routerRegistrar func(*gin.Engine)
}

// GinConfig Gin配置
type GinConfig struct {
	Port            string
	Mode            string
	ShutdownTimeout time.Duration
}

// GinModeForEnv 根据环境变量设置Gin模式
func GinModeForEnv(env string) string {
	switch env {
	case constant.EnvLocal:
		return gin.DebugMode
	case constant.EnvTest:
		return gin.TestMode
	case constant.EnvProd:
		return gin.ReleaseMode
	default:
		return gin.ReleaseMode
	}
}

// WithGinPort 设置Gin端口
func WithGinPort(port string) GinOption {
	return func(g *GinComponent) {
		g.config.Port = port
	}
}

// WithGinMode 设置Gin模式
func WithGinMode(mode string) GinOption {
	return func(g *GinComponent) {
		g.config.Mode = mode
	}
}

// WithGinShutdownTimeout 设置关闭超时时间
func WithGinShutdownTimeout(timeout time.Duration) GinOption {
	return func(g *GinComponent) {
		g.config.ShutdownTimeout = timeout
	}
}

// WithGinRouter 设置路由注册函数
func WithGinRouter(routerRegistrar func(*gin.Engine)) GinOption {
	return func(g *GinComponent) {
		g.routerRegistrar = routerRegistrar
	}
}

// NewGinComponent 创建Gin组件
func NewGinComponent(opts ...GinOption) *GinComponent {
	g := &GinComponent{
		config: &GinConfig{
			Port:            "8080",
			Mode:            gin.DebugMode,
			ShutdownTimeout: 5 * time.Second,
		},
	}

	for _, opt := range opts {
		opt(g)
	}

	// 设置Gin模式
	gin.SetMode(g.config.Mode)
	// 创建Gin引擎
	g.engine = gin.Default()

	// 框架基前置中间件
	g.engine.Use(
		middleware.ContextMiddleware(), // 添加上下文中间件
		// middleware.RecoveryMiddleware(), // 全局panic中间件
	)

	return g
}

// Start 启动Gin组件
func (g *GinComponent) Start(ctx context.Context) error {
	// 注册路由
	if g.routerRegistrar != nil {
		g.routerRegistrar(g.engine)
	}

	// 在端口前面拼接":"
	serverPort := ":" + g.config.Port
	// 创建HTTP服务器
	g.server = &http.Server{
		Addr:    serverPort, // 设置监听地址
		Handler: g.engine,   // 设置处理请求的handler
	}

	// 设置受信任的代理
	g.engine.SetTrustedProxies([]string{"0.0.0.0/0"})

	// 启动HTTP服务器
	go func() {
		//启动服务
		fmt.Printf("服务启动 - 端口 %s\n", serverPort)
		if err := g.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("Gin server error: %v", err)
		}
	}()

	return nil
}

// Stop 停止Gin组件
func (g *GinComponent) Stop(ctx context.Context) error {
	fmt.Printf("停止服务: %s\n", g.config.Port)
	if g.server != nil {
		// 创建带超时的上下文
		shutdownCtx, cancel := context.WithTimeout(ctx, g.config.ShutdownTimeout)
		defer cancel()

		// 优雅关闭
		return g.server.Shutdown(shutdownCtx)
	}
	return nil
}

// GetEngine 获取Gin引擎
func (g *GinComponent) GetEngine() *gin.Engine {
	return g.engine
}
