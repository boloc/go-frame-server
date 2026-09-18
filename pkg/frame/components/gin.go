package components

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/boloc/go-frame-server/v2/pkg/alert"
	"github.com/boloc/go-frame-server/v2/pkg/constant"
	"github.com/boloc/go-frame-server/v2/pkg/frame/middleware"
	"github.com/boloc/go-frame-server/v2/pkg/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
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

	// warnedForwardedHeader 记录 TrustedProxies 未配置警告是否已打印，避免逐请求刷屏。
	warnedForwardedHeader atomic.Bool

	// registerRoutesOnce 保证 routerRegistrar 只在这个组件的生命周期里执行一次——
	// Restart（Stop 再 Start）用的是同一个 g.engine，重复注册同一批路径 gin 会 panic。
	registerRoutesOnce sync.Once
}

// GinConfig Gin 配置。HTTP 超时有安全默认值；TrustedProxies 必须在生产环境显式声明，见字段注释。
type GinConfig struct {
	Port            string
	Mode            string
	ShutdownTimeout time.Duration

	// HTTP 层超时，映射到 http.Server 同名字段；默认值见 NewGinComponent。
	ReadHeaderTimeout time.Duration // 读取请求头的超时
	ReadTimeout       time.Duration // 读取整个请求（含 body）的超时
	WriteTimeout      time.Duration // 写响应的超时
	IdleTimeout       time.Duration // keep-alive 连接的空闲超时
	MaxHeaderBytes    int           // 单个请求 header 的最大字节数

	// TrustedProxies 决定 c.ClientIP() 解析 X-Forwarded-For 时信任哪些来源。
	//
	// nil（未显式配置）：非 Release 模式允许启动并 Warn 一次；gin.ReleaseMode 下 Start 返回 error。
	// 非 nil 空切片：显式声明直连、不信任任何代理（对 gin.SetTrustedProxies 传 nil）。
	// 非空列表：信任这些 CIDR。部署在网关后应配网关网段；不要配 "0.0.0.0/0"。
	//
	// 调用方用 viper.IsSet("server.trusted_proxies") 判断是否显式配置：
	// 显式配置了就传 []string{} 或具体 CIDR；没配就不传（保持 nil）。
	// WithGinTrustedProxies(nil) 与不调用等价。
	TrustedProxies []string

	// MaxBodyBytes 限制单个请求体大小（字节），0 表示不限制。默认 4MB。
	MaxBodyBytes int64

	// RequestTimeout 给每个请求的 context 挂整体超时，0 表示不启用（默认）。
	// 需 handler/logic 把 c.Request.Context() 传给下游才会生效。
	RequestTimeout time.Duration
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

// WithGinReadHeaderTimeout 设置读取请求头的超时。
func WithGinReadHeaderTimeout(timeout time.Duration) GinOption {
	return func(g *GinComponent) {
		g.config.ReadHeaderTimeout = timeout
	}
}

// WithGinReadTimeout 设置读取整个请求的超时。
func WithGinReadTimeout(timeout time.Duration) GinOption {
	return func(g *GinComponent) {
		g.config.ReadTimeout = timeout
	}
}

// WithGinWriteTimeout 设置写响应的超时。
func WithGinWriteTimeout(timeout time.Duration) GinOption {
	return func(g *GinComponent) {
		g.config.WriteTimeout = timeout
	}
}

// WithGinIdleTimeout 设置 keep-alive 连接的空闲超时。
func WithGinIdleTimeout(timeout time.Duration) GinOption {
	return func(g *GinComponent) {
		g.config.IdleTimeout = timeout
	}
}

// WithGinMaxHeaderBytes 设置单个请求 header 的最大字节数。
func WithGinMaxHeaderBytes(n int) GinOption {
	return func(g *GinComponent) {
		g.config.MaxHeaderBytes = n
	}
}

// WithGinTrustedProxies 设置信任的代理来源（CIDR 列表）。
// 传 nil 或不调用等价：未显式配置。生产环境（gin.ReleaseMode）Start 会因此失败。
// 传非 nil 空切片表示显式确认直连、不信任任何代理。
func WithGinTrustedProxies(cidrs []string) GinOption {
	return func(g *GinComponent) {
		g.config.TrustedProxies = cidrs
	}
}

// WithGinMaxBodyBytes 设置请求体大小上限（字节），传 0 关闭限制。默认 4MB。
func WithGinMaxBodyBytes(n int64) GinOption {
	return func(g *GinComponent) {
		g.config.MaxBodyBytes = n
	}
}

// WithGinRequestTimeout 设置每个请求的整体超时，传 0（默认）关闭。
// 需 handler/logic 把 c.Request.Context() 传给下游才会生效。
func WithGinRequestTimeout(d time.Duration) GinOption {
	return func(g *GinComponent) {
		g.config.RequestTimeout = d
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

// NewGinComponent 创建 Gin 组件。HTTP 超时带安全默认值；TrustedProxies 默认 nil（未显式配置）。
func NewGinComponent(opts ...GinOption) *GinComponent {
	g := &GinComponent{
		config: &GinConfig{
			Port:            "8080",
			Mode:            gin.DebugMode,
			ShutdownTimeout: 5 * time.Second,

			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       60 * time.Second,
			MaxHeaderBytes:    1 << 20, // 1MB
			TrustedProxies:    nil,     // 未显式配置，见 GinConfig 字段注释
			MaxBodyBytes:      4 << 20, // 4MB，见 GinConfig 字段注释
			RequestTimeout:    0,       // 默认不启用，见 GinConfig 字段注释
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
	// 添加恢复中间件，但不添加日志中间件。
	// 不用 gin.Recovery()：它把堆栈打到 os.Stderr（DefaultErrorWriter），跟 pkg/logger
	// 完全是两条通道——生产环境如果只采集应用日志文件、不采集容器标准错误流，panic 现场
	// 会直接丢失。这里换成 CustomRecoveryWithWriter(nil, ...)：nil 关闭 gin 内置的 stderr
	// 输出，堆栈改成经 pkg/logger 落地（同时触发 alert.Notify），响应行为不变（500，无 body）。
	g.engine.Use(gin.CustomRecoveryWithWriter(nil, g.recoverAndLog))

	// 请求体限制和请求超时须在会读整包 body 的中间件之前生效，故在此先 Use。
	if g.config.MaxBodyBytes > 0 {
		g.engine.Use(middleware.MaxBodyBytes(g.config.MaxBodyBytes))
	}
	if g.config.RequestTimeout > 0 {
		g.engine.Use(middleware.RequestTimeout(g.config.RequestTimeout))
	}

	// TrustedProxies 未配置时检测转发头，避免配置遗漏完全静默。
	if g.config.TrustedProxies == nil {
		g.engine.Use(g.detectUnconfiguredProxy())
	}

	// 应用用户配置的中间件
	if len(g.middlewares) > 0 {
		g.engine.Use(g.middlewares...)
	}

	return g
}

// recoverAndLog 是 gin panic 恢复后的处理函数：完整堆栈落进 pkg/logger + alert.Notify，
// 响应仍然是 500 + 空 body（跟 gin 默认 Recovery 行为一致，不在这里引入 webx 的响应格式，
// 保持 components 这一层不依赖更上层的响应约定）。
func (g *GinComponent) recoverAndLog(c *gin.Context, recovered any) {
	logger.Error("gin: panic recovered",
		zap.Any("panic", recovered),
		zap.String("route", c.FullPath()),
		zap.String("method", c.Request.Method),
		zap.String("stack", string(debug.Stack())),
	)
	alert.Notify(c.Request.Context(), alert.Event{
		Scope: "gin", Name: "panic",
		Message: fmt.Sprintf("panic recovered: route=%s method=%s", c.FullPath(), c.Request.Method),
	})
	c.AbortWithStatus(http.StatusInternalServerError)
}

// trustedProxiesForGin 把配置值转成 gin.SetTrustedProxies 的参数。
// 非 nil 空切片表示显式直连，gin 语义是传 nil（不信任任何代理、不解析转发头）。
func trustedProxiesForGin(proxies []string) []string {
	if len(proxies) == 0 {
		return nil
	}
	return proxies
}

// detectUnconfiguredProxy 在 TrustedProxies 未配置且请求带转发头时警告一次，不影响请求处理。
func (g *GinComponent) detectUnconfiguredProxy() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !g.warnedForwardedHeader.Load() {
			if c.GetHeader("X-Forwarded-For") != "" || c.GetHeader("X-Real-IP") != "" {
				if g.warnedForwardedHeader.CompareAndSwap(false, true) {
					logger.Warn("[gin] 收到带 X-Forwarded-For/X-Real-IP 的请求，但 TrustedProxies 未配置，" +
						"该请求头会被忽略，c.ClientIP() 返回的是直连的 TCP 对端地址。" +
						"如果这个服务确实部署在 Nginx/CDN/负载均衡后面，所有请求会拿到同一个代理 IP；" +
						"请用 components.WithGinTrustedProxies 配置代理网段。如果这个服务是直连对外的，可以忽略这条提示。")
				}
			}
		}
		c.Next()
	}
}

// Start 启动Gin组件
func (g *GinComponent) Start(ctx context.Context) error {
	// 生产环境必须显式声明 TrustedProxies：nil 表示调用方忘了配，空切片表示确认直连。
	if g.config.Mode == gin.ReleaseMode && g.config.TrustedProxies == nil {
		return errors.New("生产环境必须显式配置 server.trusted_proxies：部署在代理后面填代理网段 CIDR；直连对外的服务显式写 `trusted_proxies: []` 表示确认")
	}

	// 注册路由：只在第一次 Start 时执行一次，见 registerRoutesOnce 的文档注释。
	g.registerRoutesOnce.Do(func() {
		if g.routerRegistrar != nil {
			g.routerRegistrar(g.engine)
		}
	})

	// []string{} 表示显式直连：对 gin 传 nil（不解析转发头，c.ClientIP() 用 TCP 对端）。
	if err := g.engine.SetTrustedProxies(trustedProxiesForGin(g.config.TrustedProxies)); err != nil {
		return fmt.Errorf("set trusted proxies: %w", err)
	}
	if g.config.TrustedProxies == nil {
		logger.Info("[gin] TrustedProxies 未配置：c.ClientIP() 只使用直连的 TCP 对端地址，不解析转发头。" +
			"部署在 Nginx/CDN/负载均衡后面时需要用 components.WithGinTrustedProxies 配置代理网段，否则所有用户会拿到同一个代理 IP。" +
			"直连对外（没有代理）的服务可以忽略这条提示。")
	}

	// 在端口前面拼接":"
	serverPort := ":" + g.config.Port

	// 同步 Listen：绑定失败时 Start 立即返回 error，Frame 会回滚已启动的组件。
	ln, err := net.Listen("tcp", serverPort)
	if err != nil {
		return fmt.Errorf("listen %s: %w", serverPort, err)
	}

	g.server = &http.Server{
		Addr:              serverPort,
		Handler:           g.engine,
		ReadHeaderTimeout: g.config.ReadHeaderTimeout,
		ReadTimeout:       g.config.ReadTimeout,
		WriteTimeout:      g.config.WriteTimeout,
		IdleTimeout:       g.config.IdleTimeout,
		MaxHeaderBytes:    g.config.MaxHeaderBytes,
	}

	// 端口已绑定，Serve 放后台。非 Shutdown 的退出会打日志并 alert.Notify。
	go func() {
		logger.Info("gin: server started", zap.String("port", serverPort))
		if err := g.server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("gin: server stopped unexpectedly", zap.Error(err))
			alert.Notify(context.Background(), alert.Event{
				Scope: "gin", Name: "serve", Message: "server stopped unexpectedly", Err: err,
			})
		}
	}()

	return nil
}

// Stop 停止Gin组件
func (g *GinComponent) Stop(ctx context.Context) error {
	logger.Info("gin: server stopping", zap.String("port", g.config.Port))
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
