package main

import (
	"context"
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
	// level := config.GetConfig().GetString("logs.log_level")
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
	dataBaseName := constant.DefaultDBName
	dbMapStr := fmt.Sprintf("database.%s", dataBaseName)
	dbMap := conf.GetStringMap(dbMapStr)
	masterDSN := util.BuildMysqlDSN(dbMap["master"].(map[string]any))

	// 组装从库dsn
	slavesDSN := []string{}
	if dbMap["slaves"] != nil {
		for _, slave := range dbMap["slaves"].([]any) {
			slavesDSN = append(slavesDSN, util.BuildMysqlDSN(slave.(map[string]any)))
		}
	}
	// 创建MySQL组件
	mysqlComponent := components.NewMySQLComponent(
		dataBaseName,
		&components.MySQLConfig{
			MasterDSN:       masterDSN,
			SlavesDSN:       slavesDSN, // 传入多个从库DSN
			MaxIdleConns:    dbMap["max_idle_conns"].(int),
			MaxOpenConns:    dbMap["max_open_conns"].(int),
			ConnMaxLifetime: conf.GetStringTimeDuration(dbMap["conn_max_lifetime"].(string)),
			Prefix:          dbMap["prefix"].(string),
			LogLevel:        components.GormLogLevelForEnv(conf.GetString("server.env")),
		},
		true, // 是否默认
	)
	f.RegisterComponent(mysqlComponent)

	// // 支持注册多个数据库(主从都可，避免一个项目包含多个数据库问题)
	// mysqlComponentAnother := components.NewMySQLComponent(
	// 	"another",
	// 	&components.MySQLConfig{
	// 		MasterDSN:       masterDSN,
	// 		SlavesDSN:       slavesDSN, // 传入多个从库DSN
	// 		MaxIdleConns:    dbMap["max_idle_conns"].(int),
	// 		MaxOpenConns:    dbMap["max_open_conns"].(int),
	// 		ConnMaxLifetime: conf.GetStringTimeDuration(dbMap["conn_max_lifetime"].(string)),
	// 		Prefix:          dbMap["prefix"].(string),
	// 		LogLevel:        components.GormLogLevelForEnv(conf.GetString("server.env")),
	// 	},
	// 	false, // 是否默认
	// )
	// f.RegisterComponent(mysqlComponentAnother)
	// // 获取从库模型结果
	// dbSlave := frame.SlaveDB("another")
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
	// 	components.WithClusterTimeout(conf.GetStringTimeDuration("redis.cluster.timeout")),                   // 设置连接超时时间
	// 	components.WithClusterMaxRetries(conf.GetInt("redis.cluster.max_retries")),                           // 设置最大重试次数
	// 	components.WithClusterMinIdleConns(conf.GetInt("redis.cluster.min_idle_conns")),                      // 设置最小空闲连接数
	// 	components.WithClusterRouteRandomly(conf.GetBool("redis.cluster.route_randomly")),                    // 设置是否随机路由
	// 	components.WithClusterMinRetryBackoff(conf.GetStringTimeDuration("redis.cluster.min_retry_backoff")), // 设置最小重试间隔时间
	// 	components.WithClusterMaxRetryBackoff(conf.GetStringTimeDuration("redis.cluster.max_retry_backoff")), // 设置最大重试间隔时间
	// 	components.WithClusterReadTimeout(conf.GetStringTimeDuration("redis.cluster.read_timeout")),          // 设置读取超时时间
	// 	components.WithClusterWriteTimeout(conf.GetStringTimeDuration("redis.cluster.write_timeout")),        // 设置写入超时时间
	// )
	// f.RegisterComponent(redisClusterComponent)

	/******************** Redis组件 end ********************/

	/******************** ClickHouse组件 start ********************/
	// 注册ClickHouse组件
	// 配置样例 - 如果未配置，请先在config文件中添加相应配置
	// clickhouseComponent := components.NewClickHouseComponent(
	// 	"shortlink",
	// 	true,
	// 	components.WithClickHouseAddress([]string{conf.GetString("clickhouse.default.addr")}),                        // 设置ClickHouse地址
	// 	components.WithClickHouseDatabase(conf.GetString("clickhouse.default.database")),                             // 设置数据库名
	// 	components.WithClickHouseUsername(conf.GetString("clickhouse.default.username")),                             // 设置用户名
	// 	components.WithClickHousePassword(conf.GetString("clickhouse.default.password")),                             // 设置密码
	// 	components.WithClickHouseMaxOpenConns(conf.GetInt("clickhouse.default.max_open_conns")),                      // 设置最大连接数
	// 	components.WithClickHouseMaxIdleConns(conf.GetInt("clickhouse.default.max_idle_conns")),                      // 设置最大空闲连接数
	// 	components.WithClickHouseConnMaxLifetime(conf.GetStringTimeDuration("clickhouse.default.conn_max_lifetime")), // 设置连接最大生命周期
	// 	components.WithClickHouseDialTimeout(conf.GetStringTimeDuration("clickhouse.default.dial_timeout")),          // 设置连接超时时间
	// 	components.WithClickHouseReadTimeout(conf.GetStringTimeDuration("clickhouse.default.read_timeout")),          // 设置读取超时时间
	// 	components.WithClickHouseCompression(clickhouse.CompressionLZ4),                                              // 设置压缩方式
	// 	components.WithClickHouseDebug(conf.GetBool("clickhouse.default.debug")),                                     // 设置调试
	// 	components.WithClickHouseProtocol(conf.GetString("clickhouse.default.protocol")),                             // 设置协议
	// )

	// f.RegisterComponent(clickhouseComponent)

	// 注册ClickHouse GORM组件
	clickhouseDSN := util.BuildClickhouseDSN(map[string]any{
		"user":     conf.GetString("clickhouse.default.username"), // 用户名
		"password": conf.GetString("clickhouse.default.password"), // 密码
		"host":     conf.GetString("clickhouse.default.host"),     // 地址
		"port":     conf.GetString("clickhouse.default.port"),     // 端口
		"name":     conf.GetString("clickhouse.default.database"), // 数据库
	})
	clickhouseGorm := components.NewClickHouseGORMComponent(
		"shortlink",
		&components.ClickHouseGORMConfig{
			DSN:             clickhouseDSN,
			MaxIdleConns:    conf.GetInt("clickhouse.default.max_idle_conns"),                              // 最大空闲连接数
			MaxOpenConns:    conf.GetInt("clickhouse.default.max_open_conns"),                              // 最大连接数
			ConnMaxLifetime: conf.GetStringTimeDuration("clickhouse.default.conn_max_lifetime"),            // 连接最大生命周期
			LogLevel:        components.GormLogLevelForEnv(conf.GetString("clickhouse.default.log_level")), // 日志等级
		},
		true, // 设为默认实例
	)
	f.RegisterComponent(clickhouseGorm)
	/******************** ClickHouse组件 end ********************/

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

	// 注册启动后的操作
	f.AfterStart(func(ctx context.Context) error {
		// 在这里执行启动后的操作
		fmt.Println("框架启动后执行1...")
		return nil
	})
	f.AfterStart(func(ctx context.Context) error {
		// 在这里执行启动后的操作
		fmt.Println("框架启动后执行2...")
		return nil
	})

	// 注册停止前的操作
	f.BeforeStop(func(ctx context.Context) error {
		// 在这里执行停止前的操作
		fmt.Println("框架停止前执行...")
		return nil
	})

	// 运行框架
	if err := f.Run(); err != nil {
		fmt.Println("Framework error", err)
		// logger.Error("Framework error", zap.Error(err))
		os.Exit(1)
	}
}
