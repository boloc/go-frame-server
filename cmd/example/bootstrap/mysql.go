package bootstrap

import (
	"github.com/boloc/go-frame-server/internal/example/constant"
	"github.com/boloc/go-frame-server/pkg/frame"
	"github.com/boloc/go-frame-server/pkg/frame/components"
	"github.com/boloc/go-frame-server/pkg/frame/config"
	"github.com/boloc/go-frame-server/pkg/util"
)

// DatabaseConfig 数据库配置结构体（mapstructure 将配置解析为结构体）。
type DatabaseConfig struct {
	Master          util.MySQLDSNConfig   `mapstructure:"master"`            // 主库配置
	Slaves          []util.MySQLDSNConfig `mapstructure:"slaves"`            // 从库配置
	MaxIdleConns    int                   `mapstructure:"max_idle_conns"`    // 设置空闲连接池中的最大连接数
	MaxOpenConns    int                   `mapstructure:"max_open_conns"`    // 设置打开数据库连接的最大数量
	ConnMaxLifetime string                `mapstructure:"conn_max_lifetime"` // 设置连接可复用的最大时间 (类型为: time.Duration)
	Prefix          string                `mapstructure:"prefix"`            // 设置表前缀
}

// buildMySQLComponent 从 database.<name> 解析配置并创建组件（不注册、不启动）。
func buildMySQLComponent(conf *config.ConfigComponent, name string, isDefault bool) *components.MySQLComponent {
	var dbConfig DatabaseConfig
	if err := conf.GetViper().UnmarshalKey("database."+name, &dbConfig); err != nil {
		panic("failed to parse database." + name + " config: " + err.Error())
	}

	masterDSN := util.BuildMysqlDSN(dbConfig.Master)

	var slavesDSN []string
	for _, slave := range dbConfig.Slaves {
		slavesDSN = append(slavesDSN, util.BuildMysqlDSN(slave))
	}

	// conn_max_lifetime 解析失败在启动时直接报错，避免静默落到默认值。
	connMaxLifetime, err := config.ParseDuration(dbConfig.ConnMaxLifetime)
	if err != nil {
		panic("failed to parse database." + name + ".conn_max_lifetime: " + err.Error())
	}

	return components.NewMySQLComponent(
		name,
		&components.MySQLConfig{
			MasterDSN:       masterDSN,
			SlavesDSN:       slavesDSN,
			MaxIdleConns:    dbConfig.MaxIdleConns,
			MaxOpenConns:    dbConfig.MaxOpenConns,
			ConnMaxLifetime: connMaxLifetime,
			Prefix:          dbConfig.Prefix,
			LogLevel:        components.GormLogLevelForEnv(conf.GetString("server.env")),
		},
		isDefault,
	)
}

// SetupMySQL 初始化默认（主业务）MySQL 实例，返回组件供 SetupMonitor 挂连接池指标。
func SetupMySQL(f *frame.Frame, conf *config.ConfigComponent) *components.MySQLComponent {
	m := buildMySQLComponent(conf, constant.MySQLDefaultDB, true)
	f.RegisterComponent(m)
	return m
}

// SetupNamedMySQL 初始化一个额外的命名 MySQL 实例，演示同一进程内多个独立数据库。
// name 对应配置 database.<name>，建议用 constant 里的常量，不要手写字符串。
func SetupNamedMySQL(f *frame.Frame, conf *config.ConfigComponent, name string) *components.MySQLComponent {
	m := buildMySQLComponent(conf, name, false)
	f.RegisterComponent(m)
	return m
}
