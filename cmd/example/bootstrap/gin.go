package bootstrap

import (
	"time"

	"github.com/boloc/go-frame-server/cmd/example/route"
	"github.com/boloc/go-frame-server/pkg/frame"
	"github.com/boloc/go-frame-server/pkg/frame/components"
	"github.com/boloc/go-frame-server/pkg/frame/config"
	"github.com/boloc/go-frame-server/pkg/frame/middleware"
)

// SetupGin 初始化Gin组件
func SetupGin(f *frame.Frame, conf *config.ConfigComponent) {
	ginComponent := components.NewGinComponent(
		components.WithGinPort(conf.GetString("server.port")),
		components.WithGinMode(components.GinModeForEnv(conf.GetString("server.env"))),
		components.WithGinShutdownTimeout(5*time.Second),
		components.WithGinRouter(route.RegisterRoutes),
	)

	// 添加全局中间件
	ginComponent.Use(middleware.ContextMiddleware())
	f.RegisterComponent(ginComponent)
}

