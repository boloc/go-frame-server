package bootstrap

import (
	"fmt"

	"github.com/boloc/go-frame-server/pkg/frame"
	"github.com/boloc/go-frame-server/pkg/frame/components"
	"github.com/boloc/go-frame-server/pkg/frame/config"
	"github.com/boloc/go-frame-server/pkg/util"
)

// ClickHouseConfig ClickHouse配置结构体
type ClickHouseConfig struct {
	Host            string `mapstructure:"host"`              // 地址
	Port            int    `mapstructure:"port"`              // 端口
	Database        string `mapstructure:"database"`          // 数据库
	Username        string `mapstructure:"username"`          // 用户名
	Password        string `mapstructure:"password"`          // 密码
	MaxIdleConns    int    `mapstructure:"max_idle_conns"`    // 最大空闲连接数
	MaxOpenConns    int    `mapstructure:"max_open_conns"`    // 最大打开连接数
	ConnMaxLifetime string `mapstructure:"conn_max_lifetime"` // 连接最大生命周期
	LogLevel        string `mapstructure:"log_level"`         // 日志级别
}

// SetupClickHouse 初始化ClickHouse组件
func SetupClickHouse(f *frame.Frame, conf *config.ConfigComponent) {
	const chName = "default_ch" // 默认ClickHouse组件名称

	// 使用结构体解析配置
	var chConfig ClickHouseConfig
	if err := conf.GetViper().UnmarshalKey("clickhouse."+chName, &chConfig); err != nil {
		panic("failed to parse clickhouse config: " + err.Error())
	}

	// 组装DSN
	clickhouseDSN := util.BuildClickhouseDSN(map[string]any{
		"user":     chConfig.Username,
		"password": chConfig.Password,
		"host":     chConfig.Host,
		"port":     fmt.Sprintf("%d", chConfig.Port),
		"name":     chConfig.Database,
	})

	// 创建ClickHouse组件
	clickhouseGorm := components.NewClickHouseGORMComponent(
		chName,
		&components.ClickHouseGORMConfig{
			DSN:             clickhouseDSN,                                        // DSN连接字符串
			MaxIdleConns:    chConfig.MaxIdleConns,                                // 最大空闲连接数
			MaxOpenConns:    chConfig.MaxOpenConns,                                // 最大打开连接数
			ConnMaxLifetime: conf.GetStringTimeDuration(chConfig.ConnMaxLifetime), // 连接最大生命周期
			LogLevel:        components.GormLogLevelForEnv(chConfig.LogLevel),     // 日志级别
		},
		true,
	)
	f.RegisterComponent(clickhouseGorm)
}
