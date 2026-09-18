package bootstrap

import (
	"fmt"
	"time"

	"github.com/boloc/go-frame-server/pkg/frame"
	"github.com/boloc/go-frame-server/pkg/frame/components"
	"github.com/boloc/go-frame-server/pkg/frame/config"
	"github.com/boloc/go-frame-server/pkg/logger"
)

// ClickHouseConfig 对应配置段 clickhouse.<name>。
// clickhouse.enabled 与实例并列，不会进入本结构体。
// LogLevel 取值与 components.GormLogLevelForEnv 一致。
type ClickHouseConfig struct {
	Host            string        `mapstructure:"host" validate:"required"`
	Port            int           `mapstructure:"port" validate:"required,min=1,max=65535"`
	Database        string        `mapstructure:"database" validate:"required"`
	Username        string        `mapstructure:"username"`
	Password        string        `mapstructure:"password"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns" validate:"min=0"`
	MaxOpenConns    int           `mapstructure:"max_open_conns" validate:"min=0"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
	DialTimeout     time.Duration `mapstructure:"dial_timeout"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	Protocol        string        `mapstructure:"protocol"`
	LogLevel        string        `mapstructure:"log_level" validate:"required,oneof=local dev test production silent"`
}

// SetupClickHouse 初始化 ClickHouse 组件。是否建连由 clickhouse.enabled 控制（默认启用）。
func SetupClickHouse(f *frame.Frame, conf *config.ConfigComponent) {
	if !clickhouseEnabled(conf) {
		logger.Info("clickhouse: disabled via config (clickhouse.enabled=false), skip connecting")
		return
	}

	const chName = "default_ch" // 默认ClickHouse组件名称

	var chConfig ClickHouseConfig
	conf.MustStrictUnmarshalKey("clickhouse."+chName, &chConfig)

	clickhouseGorm := components.NewClickHouseGORMComponent(
		chName,
		&components.ClickHouseGORMConfig{
			Address:         []string{fmt.Sprintf("%s:%d", chConfig.Host, chConfig.Port)},
			Database:        chConfig.Database,
			Username:        chConfig.Username,
			Password:        chConfig.Password,
			DialTimeout:     chConfig.DialTimeout,
			ReadTimeout:     chConfig.ReadTimeout,
			Protocol:        chConfig.Protocol,
			MaxIdleConns:    chConfig.MaxIdleConns,
			MaxOpenConns:    chConfig.MaxOpenConns,
			ConnMaxLifetime: chConfig.ConnMaxLifetime,
			LogLevel:        components.GormLogLevelForEnv(chConfig.LogLevel),
		},
		true,
	)
	f.RegisterComponent(clickhouseGorm)
}

// clickhouseEnabled 读取 clickhouse.enabled；未配置时默认启用。
func clickhouseEnabled(conf *config.ConfigComponent) bool {
	if !conf.IsSet("clickhouse.enabled") {
		return true
	}
	return conf.GetBool("clickhouse.enabled")
}
