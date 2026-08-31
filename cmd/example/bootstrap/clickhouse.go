package bootstrap

import (
	"fmt"

	"github.com/boloc/go-frame-server/pkg/frame"
	"github.com/boloc/go-frame-server/pkg/frame/components"
	"github.com/boloc/go-frame-server/pkg/frame/config"
	"github.com/boloc/go-frame-server/pkg/logger"
)

// ClickHouseConfig ClickHouse 配置结构体，字段直接对应 ClickHouseGORMConfig。
type ClickHouseConfig struct {
	Host            string `mapstructure:"host"`              // 地址
	Port            int    `mapstructure:"port"`              // 端口
	Database        string `mapstructure:"database"`          // 数据库
	Username        string `mapstructure:"username"`          // 用户名
	Password        string `mapstructure:"password"`          // 密码
	MaxIdleConns    int    `mapstructure:"max_idle_conns"`    // 最大空闲连接数
	MaxOpenConns    int    `mapstructure:"max_open_conns"`    // 最大打开连接数
	ConnMaxLifetime string `mapstructure:"conn_max_lifetime"` // 连接最大生命周期
	DialTimeout     string `mapstructure:"dial_timeout"`      // 连接超时时间
	ReadTimeout     string `mapstructure:"read_timeout"`      // 读取超时时间
	Protocol        string `mapstructure:"protocol"`          // native（默认）或 http
	LogLevel        string `mapstructure:"log_level"`         // 日志级别
}

// SetupClickHouse 初始化 ClickHouse 组件。是否建连由 clickhouse.enabled 控制（默认启用）。
func SetupClickHouse(f *frame.Frame, conf *config.ConfigComponent) {
	if !clickhouseEnabled(conf) {
		logger.Info("clickhouse: disabled via config (clickhouse.enabled=false), skip connecting")
		return
	}

	const chName = "default_ch" // 默认ClickHouse组件名称

	// 使用结构体解析配置
	var chConfig ClickHouseConfig
	if err := conf.GetViper().UnmarshalKey("clickhouse."+chName, &chConfig); err != nil {
		panic("failed to parse clickhouse config: " + err.Error())
	}

	connMaxLifetime, err := config.ParseDuration(chConfig.ConnMaxLifetime)
	if err != nil {
		panic("failed to parse clickhouse." + chName + ".conn_max_lifetime: " + err.Error())
	}
	dialTimeout, err := config.ParseDuration(chConfig.DialTimeout)
	if err != nil {
		panic("failed to parse clickhouse." + chName + ".dial_timeout: " + err.Error())
	}
	readTimeout, err := config.ParseDuration(chConfig.ReadTimeout)
	if err != nil {
		panic("failed to parse clickhouse." + chName + ".read_timeout: " + err.Error())
	}

	// 创建ClickHouse组件
	clickhouseGorm := components.NewClickHouseGORMComponent(
		chName,
		&components.ClickHouseGORMConfig{
			Address:         []string{fmt.Sprintf("%s:%d", chConfig.Host, chConfig.Port)},
			Database:        chConfig.Database,
			Username:        chConfig.Username,
			Password:        chConfig.Password,
			DialTimeout:     dialTimeout,
			ReadTimeout:     readTimeout,
			Protocol:        chConfig.Protocol,
			MaxIdleConns:    chConfig.MaxIdleConns,                            // 最大空闲连接数
			MaxOpenConns:    chConfig.MaxOpenConns,                            // 最大打开连接数
			ConnMaxLifetime: connMaxLifetime,                                  // 连接最大生命周期
			LogLevel:        components.GormLogLevelForEnv(chConfig.LogLevel), // 日志级别
		},
		true,
	)
	f.RegisterComponent(clickhouseGorm)
}

// clickhouseEnabled 读取 clickhouse.enabled；未配置时默认启用。
func clickhouseEnabled(conf *config.ConfigComponent) bool {
	if !conf.GetViper().IsSet("clickhouse.enabled") {
		return true
	}
	return conf.GetBool("clickhouse.enabled")
}
