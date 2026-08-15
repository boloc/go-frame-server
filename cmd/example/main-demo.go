package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/boloc/go-frame-server/cmd/example/bootstrap"
	"github.com/boloc/go-frame-server/pkg/errs"
	"github.com/boloc/go-frame-server/pkg/frame"
	"github.com/boloc/go-frame-server/pkg/frame/config"
	"github.com/boloc/go-frame-server/pkg/frame/webx"
)

// main 展示框架推荐的装配顺序：先显式解析并加载配置，再创建 Frame、注册组件、运行。
// 配置路径由 config.Resolve 决定：-c 优先，其次 CONFIG_FILE，最后是这里的默认路径。
func main() {
	cfgPath := config.Resolve("./config/frame-server.yml")
	conf, err := config.LoadFile(cfgPath)
	if err != nil {
		fmt.Printf("加载配置文件失败(%s): %v\n", cfgPath, err)
		os.Exit(1)
	}

	// 创建框架实例：只负责组件生命周期和信号响应。
	f := frame.New(
		frame.WithShutdownTimeout(30 * time.Second),
	)

	// webx.Fail 默认 HTTP 200（业务码在 body.code）；OnFail 演示业务错误钩子。
	webx.SetOnFail(func(route string, err *errs.Error) {
		fmt.Printf("[webx.OnFail] route=%s code=%d message=%s\n", route, err.Code, err.Message)
	})

	// 装配所有组件（日志、MySQL、Redis、ClickHouse、Gin……）。
	bootstrap.Setup(f, conf)

	// 启动后执行一次性初始化（预热缓存、注册指标采集器等）。
	f.AfterStart(func(ctx context.Context) error {
		fmt.Println("框架启动后执行...")
		return nil
	})

	// 停止前执行清理（关闭自定义资源、flush 缓冲等）。
	f.BeforeStop(func(ctx context.Context) error {
		fmt.Println("框架停止前执行...")
		return nil
	})

	// 运行框架：阻塞直到 SIGINT/SIGTERM 优雅退出；SIGHUP 会重启组件但不退出进程。
	if err := f.Run(); err != nil {
		fmt.Println("Framework error", err)
		os.Exit(1)
	}
}
