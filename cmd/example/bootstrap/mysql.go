package bootstrap

import (
	"fmt"
	"time"

	"github.com/boloc/go-frame-server/v2/internal/example/constant"
	"github.com/boloc/go-frame-server/v2/pkg/frame"
	"github.com/boloc/go-frame-server/v2/pkg/frame/components"
	"github.com/boloc/go-frame-server/v2/pkg/frame/config"
	"github.com/boloc/go-frame-server/v2/pkg/util"
)

// DatabaseConfig 对应配置段 database.<name>。
// database.auto_migrate 与各实例并列，不会进入本结构体。
type DatabaseConfig struct {
	Master          util.MySQLDSNConfig   `mapstructure:"master" validate:"required"`
	Slaves          []util.MySQLDSNConfig `mapstructure:"slaves"`
	MaxIdleConns    int                   `mapstructure:"max_idle_conns" validate:"min=1"`
	MaxOpenConns    int                   `mapstructure:"max_open_conns" validate:"min=1"`
	ConnMaxLifetime time.Duration         `mapstructure:"conn_max_lifetime"`
	ConnMaxIdleTime time.Duration         `mapstructure:"conn_max_idle_time"`
	Prefix          string                `mapstructure:"prefix"`
}

// buildMySQLComponent 从 database.<name> 解析配置并创建组件（不注册、不启动）。
func buildMySQLComponent(conf *config.ConfigComponent, name string, isDefault bool) *components.MySQLComponent {
	var dbConfig DatabaseConfig
	conf.MustStrictUnmarshalKey("database."+name, &dbConfig)

	// DSN 拼装失败（缺 host/name/user、非法 loc/timeout）直接让启动失败。
	masterDSN, err := util.BuildMysqlDSN(dbConfig.Master)
	if err != nil {
		panic("database." + name + ".master: " + err.Error())
	}

	var slavesDSN []string
	for i, slave := range dbConfig.Slaves {
		dsn, err := util.BuildMysqlDSN(slave)
		if err != nil {
			panic(fmt.Sprintf("database.%s.slaves[%d]: %v", name, i, err))
		}
		slavesDSN = append(slavesDSN, dsn)
	}

	var serverCfg ServerConfig
	conf.MustStrictUnmarshalKey("server", &serverCfg)

	return components.NewMySQLComponent(
		name,
		&components.MySQLConfig{
			MasterDSN:       masterDSN,
			SlavesDSN:       slavesDSN,
			MaxIdleConns:    dbConfig.MaxIdleConns,
			MaxOpenConns:    dbConfig.MaxOpenConns,
			ConnMaxLifetime: dbConfig.ConnMaxLifetime,
			ConnMaxIdleTime: dbConfig.ConnMaxIdleTime,
			Prefix:          dbConfig.Prefix,
			LogLevel:        components.GormLogLevelForEnv(serverCfg.Env),
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
