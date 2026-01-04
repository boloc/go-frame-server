package bootstrap

import (
	"github.com/boloc/go-frame-server/pkg/frame"
)

// Setup 初始化所有组件
func Setup(f *frame.Frame) {
	conf := f.Config()

	// 初始化日志
	SetupLogger(f, conf)

	// 初始化MySQL
	SetupMySQL(f, conf)

	// 初始化Redis
	SetupRedis(f, conf)

	// 初始化ClickHouse
	SetupClickHouse(f, conf)

	// 初始化Gin
	SetupGin(f, conf)
}
