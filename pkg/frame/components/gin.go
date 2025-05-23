package components

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/boloc/go-frame-server/pkg/constant"

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
	// 全局中间件
	middlewares []gin.HandlerFunc
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

// WithGinMiddleware 添加全局中间件
func WithGinMiddleware(middleware ...gin.HandlerFunc) GinOption {
	return func(g *GinComponent) {
		g.middlewares = append(g.middlewares, middleware...)
	}
}

// Use 添加全局中间件
func (g *GinComponent) Use(middleware ...gin.HandlerFunc) {
	if g.engine != nil {
		g.engine.Use(middleware...)
	}
	g.middlewares = append(g.middlewares, middleware...)
}

// NewGinComponent 创建Gin组件
func NewGinComponent(opts ...GinOption) *GinComponent {
	g := &GinComponent{
		config: &GinConfig{
			Port:            "8080",
			Mode:            gin.DebugMode,
			ShutdownTimeout: 5 * time.Second,
		},
		middlewares: make([]gin.HandlerFunc, 0),
	}

	for _, opt := range opts {
		opt(g)
	}

	// 设置Gin模式
	gin.SetMode(g.config.Mode)
	// 创建Gin引擎
	g.engine = gin.New()

	if g.config.Mode == gin.DebugMode {
		// 添加日志中间件
		g.engine.Use(gin.Logger())
	}
	// 添加恢复中间件，但不添加日志中间件
	g.engine.Use(gin.Recovery())

	// 应用用户配置的中间件
	if len(g.middlewares) > 0 {
		g.engine.Use(g.middlewares...)
	}

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
		fmt.Printf("server start - port %s\n", serverPort)
		if err := g.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("Gin server error: %v", err)
		}
	}()

	return nil
}

// Stop 停止Gin组件
func (g *GinComponent) Stop(ctx context.Context) error {
	fmt.Printf("server stop - port %s\n", g.config.Port)
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
