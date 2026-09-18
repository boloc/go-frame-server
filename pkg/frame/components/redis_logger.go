package components

import (
	"context"
	"fmt"
	"sync"

	"github.com/boloc/go-frame-server/pkg/logger"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// redisZapLogger 把 go-redis 内部日志接到 pkg/logger。
// go-redis 只在异常路径打日志（连接池拨号失败、哨兵发现失败等），一律按 Warn。
type redisZapLogger struct{}

func (redisZapLogger) Printf(_ context.Context, format string, v ...any) {
	// go-redis 的消息本身已带 "redis: " 前缀，这里不再重复加。
	logger.Warn(fmt.Sprintf(format, v...), zap.String("component", "redis"))
}

var redisLoggerOnce sync.Once

// setRedisLoggerOnce 把 go-redis 的 redis.SetLogger 接到 redisZapLogger，进程内只执行一次。
func setRedisLoggerOnce() {
	redisLoggerOnce.Do(func() {
		redis.SetLogger(redisZapLogger{})
	})
}
