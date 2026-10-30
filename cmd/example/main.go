package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/boloc/go-frame-server/v2/cmd/example/bootstrap"
	"github.com/boloc/go-frame-server/v2/cmd/example/route"
	"github.com/boloc/go-frame-server/v2/pkg/alert"
	"github.com/boloc/go-frame-server/v2/pkg/constant"
	"github.com/boloc/go-frame-server/v2/pkg/frame"
	"github.com/boloc/go-frame-server/v2/pkg/frame/components"
	"github.com/boloc/go-frame-server/v2/pkg/frame/config"
	"github.com/boloc/go-frame-server/v2/pkg/frame/webx"
	"github.com/boloc/go-frame-server/v2/pkg/logger"
)

// main 展示框架推荐的装配顺序：先显式解析并加载配置，再创建 Frame、注册组件、运行。
//
// 配置路径由 config.Resolve 决定：-c/--config 优先，其次 CONFIG_FILE，最后是这里的默认路径。
// LoadFile 失败返回 error，不设全局单例；各配置段由 bootstrap 里的
// conf.MustStrictUnmarshalKey 解析——未知字段或必填缺失会直接让启动失败。
func main() {
	cfgPath := config.Resolve("./config/frame-server.yml")
	conf, err := config.LoadFile(cfgPath)
	if err != nil {
		fmt.Printf("加载配置文件失败(%s): %v\n", cfgPath, err)
		os.Exit(1)
	}

	// Frame 只负责组件生命周期和信号响应，不持有业务配置。
	// SIGHUP 热重启默认关闭。需要时加上 frame.WithRestartSignal(true)：
	// 不重读配置、重启期间 HTTP 端口会短暂关闭、AfterStart/BeforeStop 不重跑；
	// 多副本请走滚动重启，不要靠 SIGHUP。
	f := frame.New(
		frame.WithShutdownTimeout(30 * time.Second),
	)

	// 本地/开发把 Caller 写进失败响应，方便对照 /test/error-source；生产保持默认 false。
	env := conf.GetString("server.env")
	if env == constant.EnvLocal || env == constant.EnvDev {
		webx.IncludeCallerInResponse.Store(true)
	}

	// 装配所有组件（日志、MySQL、Redis、ClickHouse、Gin……）。
	bootstrap.Setup(f, conf)

	// 统一失败通知：框架各处的 alert.Notify 都会进这个 Hook。触发方已经打过日志，
	// Hook 只负责发外部渠道（飞书/企业微信……），不要再写一遍日志。示例没有接渠道。
	alert.SetHook(func(_ context.Context, e alert.Event) {
		if e.Scope == "lifecycle" {
			logger.Info("alert: event", logger.String("name", e.Name), logger.String("message", e.Message))
		}
	})

	// 启动后：打印已注册路由数，并确认进程运行所必需的依赖已经连上。
	f.AfterStart(func(ctx context.Context) error {
		_, mysqlOK := frame.TryDefaultDB()
		_, redisOK := frame.TryGetRedisCmdable()
		_, chOK := components.TryDefaultClickHouseDB()
		logger.Info("example: after start",
			logger.Int("routes", route.RegisteredRouteCount()),
			logger.Bool("mysql_ready", mysqlOK),
			logger.Bool("redis_ready", redisOK),
			logger.Bool("clickhouse_ready", chOK),
		)
		if !mysqlOK || !redisOK {
			return fmt.Errorf("after start: required deps not ready mysql=%v redis=%v", mysqlOK, redisOK)
		}
		return nil
	})

	// 停止前发一条下线通知。Notify 是异步的，不保证进程退出前一定投递完成。
	f.BeforeStop(func(ctx context.Context) error {
		alert.Notify(ctx, alert.Event{
			Scope:   "lifecycle",
			Name:    "example",
			Message: "服务下线",
		})
		logger.Info("example: before stop")
		return nil
	})

	// 阻塞直到 SIGINT/SIGTERM 优雅退出。SIGHUP 默认忽略，见上面 WithRestartSignal。
	if err := f.Run(); err != nil {
		fmt.Println("Framework error", err)
		os.Exit(1)
	}
}
