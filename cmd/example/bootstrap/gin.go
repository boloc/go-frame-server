package bootstrap

import (
	"time"

	"github.com/boloc/go-frame-server/cmd/example/route"
	"github.com/boloc/go-frame-server/pkg/frame"
	"github.com/boloc/go-frame-server/pkg/frame/components"
	"github.com/boloc/go-frame-server/pkg/frame/config"
	"github.com/boloc/go-frame-server/pkg/frame/middleware"

	"github.com/gin-gonic/gin"
)

// SetupGin 初始化 Gin 组件。metricsHandlers 是 /metrics 端点的 handler 链，不需要时传 nil。
func SetupGin(f *frame.Frame, conf *config.ConfigComponent, metricsHandlers []gin.HandlerFunc) {
	ginComponent := components.NewGinComponent(
		components.WithGinPort(conf.GetString("server.port")),
		components.WithGinMode(components.GinModeForEnv(conf.GetString("server.env"))),
		components.WithGinShutdownTimeout(5*time.Second),
		components.WithGinRouter(func(r *gin.Engine) {
			route.RegisterRoutes(r, metricsHandlers)
		}),
	)

	// 添加全局中间件
	ginComponent.Use(middleware.ContextMiddleware())
	f.RegisterComponent(ginComponent)
}
