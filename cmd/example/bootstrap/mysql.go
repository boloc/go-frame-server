package bootstrap

import (
	"github.com/boloc/go-frame-server/pkg/frame"
	"github.com/boloc/go-frame-server/pkg/frame/components"
	"github.com/boloc/go-frame-server/pkg/frame/config"
	"github.com/boloc/go-frame-server/pkg/util"
)

// DatabaseConfig 数据库配置结构体(mapstructure:将map转为结构体)
type DatabaseConfig struct {
	Master          map[string]any   `mapstructure:"master"`            // 主库配置
	Slaves          []map[string]any `mapstructure:"slaves"`            // 从库配置
	MaxIdleConns    int              `mapstructure:"max_idle_conns"`    // 设置空闲连接池中的最大连接数
	MaxOpenConns    int              `mapstructure:"max_open_conns"`    // 设置打开数据库连接的最大数量
	ConnMaxLifetime string           `mapstructure:"conn_max_lifetime"` // 设置连接可复用的最大时间 (类型为: time.Duration)
	Prefix          string           `mapstructure:"prefix"`            // 设置表前缀
}

// SetupMySQL 初始化MySQL组件
func SetupMySQL(f *frame.Frame, conf *config.ConfigComponent) {
	const dbName = "default_db"

	// 使用结构体解析配置，避免大量类型断言
	var dbConfig DatabaseConfig
	if err := conf.GetViper().UnmarshalKey("database."+dbName, &dbConfig); err != nil {
		panic("failed to parse database config: " + err.Error())
	}

	// 组装主库DSN
	masterDSN := util.BuildMysqlDSN(dbConfig.Master)

	// 组装从库DSN
	var slavesDSN []string
	for _, slave := range dbConfig.Slaves {
		slavesDSN = append(slavesDSN, util.BuildMysqlDSN(slave))
	}

	// 创建MySQL组件
	mysqlComponent := components.NewMySQLComponent(
		dbName,
		&components.MySQLConfig{
			MasterDSN:       masterDSN,
			SlavesDSN:       slavesDSN,
			MaxIdleConns:    dbConfig.MaxIdleConns,
			MaxOpenConns:    dbConfig.MaxOpenConns,
			ConnMaxLifetime: config.ParseDuration(dbConfig.ConnMaxLifetime),
			Prefix:          dbConfig.Prefix,
			LogLevel:        components.GormLogLevelForEnv(conf.GetString("server.env")),
		},
		true, // 是否设置为默认实例
	)
	f.RegisterComponent(mysqlComponent)
}
