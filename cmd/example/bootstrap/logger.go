package bootstrap

import (
	"github.com/boloc/go-frame-server/v2/pkg/frame"
	"github.com/boloc/go-frame-server/v2/pkg/frame/config"
	"github.com/boloc/go-frame-server/v2/pkg/logger"
)

// LogsConfig 对应配置段 logs。
type LogsConfig struct {
	LogLevel   string `mapstructure:"log_level" validate:"required,oneof=debug info warn error"`
	IsStdout   bool   `mapstructure:"is_stdout"`
	StdoutJSON bool   `mapstructure:"stdout_json"`
	IsFile     bool   `mapstructure:"is_file"`
	FileName   string `mapstructure:"file_name"`
	MaxSize    int    `mapstructure:"max_size" validate:"min=0"`
	MaxBackups int    `mapstructure:"max_backups" validate:"min=0"`
	MaxAge     int    `mapstructure:"max_age" validate:"min=0"`
	Compress   bool   `mapstructure:"compress"`
}

// SetupLogger 初始化日志组件，应在 bootstrap.Setup 里最先注册。
func SetupLogger(f *frame.Frame, conf *config.ConfigComponent) {
	var cfg LogsConfig
	conf.MustStrictUnmarshalKey("logs", &cfg)

	// WithLoggerStdoutJSON：容器采集 stdout 时打开，避免 ANSI 颜色码混进日志系统。
	// 字段化用法见 GET /test/logger（logger.Info + zap.String）。
	log := logger.NewLoggerComponent(
		logger.WithLoggerLevel(cfg.LogLevel),
		logger.WithLoggerStdout(cfg.IsStdout),
		logger.WithLoggerStdoutJSON(cfg.StdoutJSON),
		logger.WithLoggerIsFile(cfg.IsFile),
		logger.WithLoggerFilename(cfg.FileName),
		logger.WithLoggerMaxSize(cfg.MaxSize),
		logger.WithLoggerMaxBackups(cfg.MaxBackups),
		logger.WithLoggerMaxAge(cfg.MaxAge),
		logger.WithLoggerCompress(cfg.Compress),
	)
	f.RegisterComponent(log)
}
