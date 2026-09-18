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
	"github.com/boloc/go-frame-server/v2/pkg/errs"
	"github.com/boloc/go-frame-server/v2/pkg/frame"
	"github.com/boloc/go-frame-server/v2/pkg/frame/components"
	"github.com/boloc/go-frame-server/v2/pkg/frame/config"
	"github.com/boloc/go-frame-server/v2/pkg/frame/webx"
	"github.com/boloc/go-frame-server/v2/pkg/logger"
	"go.uber.org/zap"
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

	// webx.Fail 默认 HTTP 200（业务码在 body.code）。OnFail 比 alert.Event 更精确
	// （能拿到 route + *errs.Error），适合做请求级错误率统计；alert 覆盖后台/异步失败。
	webx.SetOnFail(func(route string, err *errs.Error) {
		logger.Warn("webx: on_fail",
			zap.String("route", route),
			zap.Int("code", int(err.Code)),
			zap.String("message", err.Message),
			zap.String("caller", err.Caller),
		)
	})

	// 本地/开发把 Caller 写进失败响应，方便对照 /test/error-source；生产保持默认 false。
	env := conf.GetString("server.env")
	if env == constant.EnvLocal || env == constant.EnvDev {
		webx.IncludeCallerInResponse.Store(true)
	}

	// 装配所有组件（日志、MySQL、Redis、ClickHouse、Gin……）。
	bootstrap.Setup(f, conf)

	// 统一失败通知：框架各处的 alert.Notify 都会进这个 Hook。
	// 真实项目按 e.Scope 分发到飞书/钉钉/Slack webhook，不要在 Hook 里同步做重活。
	// Dropped() 是并发 Hook 打满 64 后丢弃的累计数，可接进 Prometheus。
	alert.SetHook(func(_ context.Context, e alert.Event) {
		fields := []zap.Field{
			zap.String("scope", e.Scope),
			zap.String("name", e.Name),
			zap.String("message", e.Message),
			zap.Error(e.Err),
			zap.Any("fields", e.Fields),
			zap.Uint64("dropped_total", alert.Dropped()),
		}
		if e.Scope == "lifecycle" {
			logger.Info("alert: event", fields...)
			return
		}
		logger.Error("alert: event", fields...)
	})

	// 启动后：打印已注册路由数，并确认进程运行所必需的依赖已经连上。
	f.AfterStart(func(ctx context.Context) error {
		_, mysqlOK := frame.TryDefaultDB()
		_, redisOK := frame.TryGetRedisCmdable()
		_, chOK := components.TryDefaultClickHouseDB()
		logger.Info("example: after start",
			zap.Int("routes", route.RegisteredRouteCount()),
			zap.Bool("mysql_ready", mysqlOK),
			zap.Bool("redis_ready", redisOK),
			zap.Bool("clickhouse_ready", chOK),
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
