package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// LoggerOption 定义日志选项函数类型
type LoggerOption func(*LoggerComponent)

var log *zap.Logger

// LoggerComponent 日志组件
type LoggerComponent struct {
	config  *LoggerConfig
	started atomic.Bool // 使用原子操作确保日志组件的状态一致性
}

const (
	DefaultFilename               = "./logs/app.log"
	DebugLevel      zapcore.Level = iota - 1
	InfoLevel
	WarnLevel
	ErrorLevel
)

// LevelToString 将自定义级别转换为字符串
func LevelToString(l zapcore.Level) string {
	switch l {
	case DebugLevel:
		return "debug"
	case InfoLevel:
		return "info"
	case WarnLevel:
		return "warn"
	case ErrorLevel:
		return "error"
	default:
		return "info"
	}
}

// LoggerConfig 日志配置
type LoggerConfig struct {
	Level      string // debug, info, warn, error
	IsStdout   bool   // 是否输出到控制台
	IsFile     bool   // 是否输出到文件
	Filename   string // 日志文件名
	MaxSize    int    // MB
	MaxBackups int    // 最大备份数
	MaxAge     int    // 天
	Compress   bool   // 是否压缩
}

// WithLoggerLevel 设置日志级别
func WithLoggerLevel(level string) LoggerOption {
	return func(l *LoggerComponent) {
		l.config.Level = level
	}
}

// WithLoggerFilename 设置日志文件名
func WithLoggerFilename(filename string) LoggerOption {
	// 如果文件名没有设置，则使用默认值
	if filename == "" {
		filename = DefaultFilename
	}

	// 如果文件名中没有包含logs目录，则自动添加
	if !strings.Contains(filename, "logs") {
		filename = "./logs/" + filename
	}

	return func(l *LoggerComponent) {
		l.config.Filename = filename
	}
}

// WithLoggerMaxSize 设置日志文件最大大小
func WithLoggerMaxSize(maxSize int) LoggerOption {
	return func(l *LoggerComponent) {
		l.config.MaxSize = maxSize
	}
}

// WithLoggerMaxBackups 设置最大备份数
func WithLoggerMaxBackups(maxBackups int) LoggerOption {
	return func(l *LoggerComponent) {
		l.config.MaxBackups = maxBackups
	}
}

// WithLoggerMaxAge 设置最大保留天数
func WithLoggerMaxAge(maxAge int) LoggerOption {
	return func(l *LoggerComponent) {
		l.config.MaxAge = maxAge
	}
}

// WithLoggerCompress 设置是否压缩
func WithLoggerCompress(compress bool) LoggerOption {
	return func(l *LoggerComponent) {
		l.config.Compress = compress
	}
}

// WithLoggerStdout 设置是否输出到控制台
func WithLoggerStdout(isStdout bool) LoggerOption {
	return func(l *LoggerComponent) {
		l.config.IsStdout = isStdout
	}
}

// WithLoggerIsFile 设置是否输出到文件
func WithLoggerIsFile(isFile bool) LoggerOption {
	return func(l *LoggerComponent) {
		l.config.IsFile = isFile
	}
}

// NewLoggerComponent 创建日志组件
func NewLoggerComponent(opts ...LoggerOption) *LoggerComponent {
	l := &LoggerComponent{
		config: &LoggerConfig{
			Level:      LevelToString(InfoLevel), // 默认值 info
			MaxSize:    100,                      // 默认值 100MB
			MaxBackups: 3,                        // 默认值 3
			MaxAge:     7,                        // 默认值 7天
			Compress:   true,                     // 默认值 true
			IsStdout:   true,                     // 默认值 true
			IsFile:     true,                     // 默认值 true
			Filename:   DefaultFilename,          // 默认值 ./logs/app.log
		},
	}

	for _, opt := range opts {
		opt(l)
	}

	return l
}

// Start 启动日志组件
func (l *LoggerComponent) Start() error {
	// 确保不会重复启动
	if l.started.Load() {
		return nil
	}

	// 设置日志级别
	var level zapcore.Level
	switch l.config.Level {
	case LevelToString(DebugLevel):
		level = zapcore.DebugLevel
	case LevelToString(InfoLevel):
		level = zapcore.InfoLevel
	case LevelToString(WarnLevel):
		level = zapcore.WarnLevel
	case LevelToString(ErrorLevel):
		level = zapcore.ErrorLevel
	default:
		level = zapcore.InfoLevel
	}

	// 配置通用编码器设置
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeTime:     timeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	// 定义输出
	var cores []zapcore.Core
	// 文件输出
	if l.config.IsFile && l.config.Filename != "" {
		// 确保日志目录存在
		dir := filepath.Dir(l.config.Filename)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("创建日志目录失败: %w", err)
		}

		// 配置lumberjack进行日志切割
		fileWriter := &lumberjack.Logger{
			Filename:   l.config.Filename,   // 日志文件名
			MaxSize:    l.config.MaxSize,    // 最大文件大小
			MaxBackups: l.config.MaxBackups, // 最大备份数
			MaxAge:     l.config.MaxAge,     // 最大保留天数
			Compress:   l.config.Compress,   // 是否压缩
			LocalTime:  true,                // 使用本地时间
		}

		// 文件输出的编码器配置 - 使用JSON格式
		fileEncoderConfig := encoderConfig
		fileEncoderConfig.EncodeLevel = zapcore.LowercaseLevelEncoder

		// 文件使用JSON格式，便于后期分析
		fileCore := zapcore.NewCore(
			zapcore.NewJSONEncoder(fileEncoderConfig),
			zapcore.AddSync(fileWriter),
			level,
		)
		cores = append(cores, fileCore)
	}

	// 控制台输出
	if l.config.IsStdout {
		// 控制台输出的编码器配置 - 使用彩色输出
		consoleEncoderConfig := encoderConfig
		consoleEncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder

		// 控制台使用更易读的格式
		consoleCore := zapcore.NewCore(
			zapcore.NewConsoleEncoder(consoleEncoderConfig),
			zapcore.AddSync(os.Stdout),
			level,
		)
		cores = append(cores, consoleCore)
	}

	// 检查是否有可用的core
	if len(cores) == 0 {
		return fmt.Errorf("没有配置任何日志输出，请设置IsFile或Stdout为true")
	}

	// 合并所有输出
	core := zapcore.NewTee(cores...)

	// 创建记录器，开启调用信息
	log = zap.New(
		core,
		zap.AddCaller(),
		zap.AddCallerSkip(1),
		zap.AddStacktrace(zapcore.ErrorLevel), // 为错误级别以上添加堆栈跟踪
		zap.WithFatalHook(zapcore.WriteThenFatal), // 确保 Fatal 级别的日志在程序退出前被写入
	)

	// 替换全局记录器
	zap.ReplaceGlobals(log)

	// 设置启动标志
	l.started.Store(true)

	return nil
}

func (l *LoggerComponent) GetLogger() *zap.Logger {
	return log
}

// 时间编码器
func timeEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(t.Format("2006-01-02 15:04:05"))
}

// 以下是一些便捷方法
func Debug(msg string, fields ...zap.Field) {
	log.Debug(msg, fields...)
}

func Info(msg string, fields ...zap.Field) {
	log.Info(msg, fields...)
}

func Warn(msg string, fields ...zap.Field) {
	log.Warn(msg, fields...)
}

func Error(msg string, fields ...zap.Field) {
	log.Error(msg, fields...)
}

func Fatal(msg string, fields ...zap.Field) {
	log.Fatal(msg, fields...)
}
