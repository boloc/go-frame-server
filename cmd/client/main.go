package main

import (
	"fmt"
	"os"
	"time"

	"github.com/boloc/go-frame-server/cmd/client/route"
	"github.com/boloc/go-frame-server/pkg/constant"
	"github.com/boloc/go-frame-server/pkg/frame"
	"github.com/boloc/go-frame-server/pkg/frame/components"
	"github.com/boloc/go-frame-server/pkg/frame/config"
	"github.com/boloc/go-frame-server/pkg/frame/middleware"
	"github.com/boloc/go-frame-server/pkg/logger"
	"github.com/boloc/go-frame-server/pkg/util"
)

func main() {
	// 创建框架实例
	f := frame.New(
		frame.WithShutdownTimeout(30 * time.Second), // 设置30秒关闭超时
	)

	// 注册配置组件
	conf := config.MustLoad("frame-server", "./config")
	// 正常获取
	// level := frame.GetConfig().GetString("logs.log_level")
	// fmt.Println("打印日志级别", level)

	/* 日志组件 */
	// 注册日志组件
	log := logger.NewLoggerComponent(
		logger.WithLoggerLevel(conf.GetString("logs.log_level")),     // 设置日志级别
		logger.WithLoggerStdout(conf.GetBool("logs.is_stdout")),      // 设置是否输出到控制台
		logger.WithLoggerIsFile(conf.GetBool("logs.is_file")),        // 设置是否输出到文件
		logger.WithLoggerFilename(conf.GetString("logs.file_name")),  // 设置文件名
		logger.WithLoggerMaxSize(conf.GetInt("logs.max_size")),       // 设置文件最大大小
		logger.WithLoggerMaxBackups(conf.GetInt("logs.max_backups")), // 设置文件最大备份数
		logger.WithLoggerMaxAge(conf.GetInt("logs.max_age")),         // 设置文件最大保存时间
		logger.WithLoggerCompress(conf.GetBool("logs.compress")),     // 设置是否压缩文件
	)
	log.Start()
	// 给框架设置日志记录器
	f.SetLogger(log.GetLogger())
	/* 日志组件 end */

	/******************** 数据库组件 start ********************/
	// 组装主库dsn
	databasename := constant.DefaultDBName
	dbMapStr := fmt.Sprintf("database.%s", databasename)
	dbMap := conf.GetStringMap(dbMapStr)
	masterDSN := util.BuildMysqlDSN(dbMap["master"].(map[string]any))

	// 组装从库dsn
	slaveDSNs := []string{}
	if dbMap["slaves"] != nil {
		for _, slave := range dbMap["slaves"].([]any) {
			slaveDSNs = append(slaveDSNs, util.BuildMysqlDSN(slave.(map[string]any)))
		}
	}
	// 创建MySQL组件
	mysqlComponent := components.NewMySQLComponent(
		databasename,
		&components.MySQLConfig{
			MasterDSN:       masterDSN,
			SlaveDSNs:       slaveDSNs, // 传入多个从库DSN
			MaxIdleConns:    dbMap["max_idle_conns"].(int),
			MaxOpenConns:    dbMap["max_open_conns"].(int),
			ConnMaxLifetime: conf.GetSrtingTimeDuration(dbMap["conn_max_lifetime"].(string)),
			Prefix:          dbMap["prefix"].(string),
			LogLevel:        components.GormLogLevelForEnv(conf.GetString("server.env")),
		},
		true, // 是否默认
	)
	f.RegisterComponent(mysqlComponent)

	// // 支持注册多个数据库(主从都可，避免一个项目包含多个数据库问题)
	// mysqlComponenta := components.NewMySQLComponent(
	// 	"a",
	// 	&components.MySQLConfig{
	// 		MasterDSN:       masterDSN,
	// 		SlaveDSNs:       slaveDSNs, // 传入多个从库DSN
	// 		MaxIdleConns:    dbMap["max_idle_conns"].(int),
	// 		MaxOpenConns:    dbMap["max_open_conns"].(int),
	// 		ConnMaxLifetime: conf.GetSrtingTimeDuration(dbMap["conn_max_lifetime"].(string)),
	// 		Prefix:          dbMap["prefix"].(string),
	// 		LogLevel:        components.GormLogLevelForEnv(conf.GetString("server.env")),
	// 	},
	// 	false, // 是否默认
	// )
	// f.RegisterComponent(mysqlComponenta)
	// 获取从库
	// dbSlave := frame.SlaveDB("a")
	// var v []map[string]any
	// dbSlave.Model(&model.ShortLinkRelationship{}).Find(&v)
	// fmt.Println("打印db结果", v)

	/******************** 数据库组件 end ********************/

	/******************** Redis组件 start ********************/
	// 注册Redis单机组件
	redisComponent := components.NewRedisComponent(
		components.WithRedisAddr(conf.GetString("redis.single.addr")),                // 设置Redis地址
		components.WithRedisPassword(conf.GetString("redis.single.password")),        // 设置Redis密码
		components.WithRedisDB(conf.GetInt("redis.single.db")),                       // 设置Redis数据库
		components.WithRedisPoolSize(conf.GetInt("redis.single.pool_size")),          // 设置Redis连接池大小
		components.WithRedisMinIdleConns(conf.GetInt("redis.single.min_idle_conns")), // 设置Redis最小空闲连接数
	)
	f.RegisterComponent(redisComponent)

	// 注册Redis集群组件
	// redisClusterComponent := components.NewRedisClusterComponent(
	// 	components.WithClusterAddrs(conf.GetStringSlice("redis.cluster.nodes")),                              // 设置集群节点
	// 	components.WithClusterPoolSize(conf.GetInt("redis.cluster.pool_size")),                               // 设置连接池大小
	// 	components.WithClusterTimeout(conf.GetSrtingTimeDuration("redis.cluster.timeout")),                   // 设置连接超时时间
	// 	components.WithClusterMaxRetries(conf.GetInt("redis.cluster.max_retries")),                           // 设置最大重试次数
	// 	components.WithClusterMinIdleConns(conf.GetInt("redis.cluster.min_idle_conns")),                      // 设置最小空闲连接数
	// 	components.WithClusterRouteRandomly(conf.GetBool("redis.cluster.route_randomly")),                    // 设置是否随机路由
	// 	components.WithClusterMinRetryBackoff(conf.GetSrtingTimeDuration("redis.cluster.min_retry_backoff")), // 设置最小重试间隔时间
	// 	components.WithClusterMaxRetryBackoff(conf.GetSrtingTimeDuration("redis.cluster.max_retry_backoff")), // 设置最大重试间隔时间
	// 	components.WithClusterReadTimeout(conf.GetSrtingTimeDuration("redis.cluster.read_timeout")),          // 设置读取超时时间
	// 	components.WithClusterWriteTimeout(conf.GetSrtingTimeDuration("redis.cluster.write_timeout")),        // 设置写入超时时间
	// )
	// f.RegisterComponent(redisClusterComponent)

	/******************** Redis组件 end ********************/

	/******************** Gin组件 start ********************/
	// 注册Gin组件
	ginComponent := components.NewGinComponent(
		components.WithGinPort(conf.GetString("server.port")),                          // 设置Gin端口
		components.WithGinMode(components.GinModeForEnv(conf.GetString("server.env"))), // 设置Gin模式
		components.WithGinShutdownTimeout(5*time.Second),                               // 设置5秒关闭超时
		components.WithGinRouter(route.RegisterRoutes),                                 // 注册路由
	)
	// 添加全局中间件
	ginComponent.Use(middleware.ContextMiddleware())
	f.RegisterComponent(ginComponent)
	/******************** Gin组件 end ********************/

	// 运行框架
	if err := f.Run(); err != nil {
		fmt.Println("Framework error", err)
		// logger.Error("Framework error", zap.Error(err))
		os.Exit(1)
	}
}
