package bootstrap

import (
	"github.com/boloc/go-frame-server/pkg/frame"
	"github.com/boloc/go-frame-server/pkg/frame/config"
	"github.com/boloc/go-frame-server/pkg/logger"
)

// SetupLogger 初始化日志组件
func SetupLogger(f *frame.Frame, conf *config.ConfigComponent) {
	log := logger.NewLoggerComponent(
		logger.WithLoggerLevel(conf.GetString("logs.log_level")),     // 日志级别
		logger.WithLoggerStdout(conf.GetBool("logs.is_stdout")),      // 是否输出到控制台
		logger.WithLoggerIsFile(conf.GetBool("logs.is_file")),        // 是否输出到文件
		logger.WithLoggerFilename(conf.GetString("logs.file_name")),  // 日志文件名
		logger.WithLoggerMaxSize(conf.GetInt("logs.max_size")),       // 日志文件最大大小
		logger.WithLoggerMaxBackups(conf.GetInt("logs.max_backups")), // 日志文件最大备份数
		logger.WithLoggerMaxAge(conf.GetInt("logs.max_age")),         // 日志文件最大保留天数
		logger.WithLoggerCompress(conf.GetBool("logs.compress")),     // 是否压缩
	)
	log.Start()
	f.SetLogger(log.GetLogger())
}
