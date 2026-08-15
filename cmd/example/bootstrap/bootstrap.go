package bootstrap

import (
	"github.com/boloc/go-frame-server/internal/example/constant"
	"github.com/boloc/go-frame-server/pkg/frame"
	"github.com/boloc/go-frame-server/pkg/frame/components"
	"github.com/boloc/go-frame-server/pkg/frame/config"
)

// Setup 初始化所有组件。conf 由 main 显式加载后传入。
func Setup(f *frame.Frame, conf *config.ConfigComponent) {
	// 设置进程时区：影响 time.Now()/日志时间戳，跟 MySQL 连接的时区无关。
	SetupTimezone(conf)

	// 初始化日志：保证后续组件 Start/Stop 时日志已就绪。
	SetupLogger(f, conf)

	// 初始化 MySQL：default_db 是主业务库；config_db/log_db 演示同一进程内多个命名实例。
	mysqlComponent := SetupMySQL(f, conf)
	configDBComponent := SetupNamedMySQL(f, conf, constant.MySQLConfigDB)
	logDBComponent := SetupNamedMySQL(f, conf, constant.MySQLLogDB)

	// 数据库自动迁移：由 database.auto_migrate 控制（默认关闭），在 MySQL Start 成功后执行。
	SetupMigration(f, conf)

	// 初始化Redis
	redisComponent := SetupRedis(f, conf)

	// 初始化ClickHouse
	SetupClickHouse(f, conf)

	// 初始化定时任务调度器，随 Frame 生命周期启动/关闭；由 cron.enabled 控制（默认启用）。
	SetupCron(f, conf)

	// 初始化这个应用的所有双层刷新只读缓存实例；具体有哪几个缓存是 refreshcache.go
	// 内部的细节，见 SetupCaches 的文档注释。
	SetupCaches(f)

	// 初始化指标采集：进程级指标 + 每个命名 MySQL/Redis 各自一份连接池采集器。
	metricsHandlers := SetupMonitor(f, conf, map[string]*components.MySQLComponent{
		constant.MySQLDefaultDB: mysqlComponent,
		constant.MySQLConfigDB:  configDBComponent,
		constant.MySQLLogDB:     logDBComponent,
	}, redisComponent)

	// 初始化对象存储（可选能力，未配置时自动跳过）。
	SetupStorage(conf)

	// 初始化 Gin，并把 /metrics handler 链传给路由。
	SetupGin(f, conf, metricsHandlers)
}
