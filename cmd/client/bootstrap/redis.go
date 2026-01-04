package bootstrap

import (
	"github.com/boloc/go-frame-server/pkg/frame"
	"github.com/boloc/go-frame-server/pkg/frame/components"
	"github.com/boloc/go-frame-server/pkg/frame/config"
)

// SetupRedis 初始化Redis单机组件
func SetupRedis(f *frame.Frame, conf *config.ConfigComponent) {
	redisComponent := components.NewRedisComponent(
		components.WithRedisAddr(conf.GetString("redis.single.addr")),
		components.WithRedisPassword(conf.GetString("redis.single.password")),
		components.WithRedisDB(conf.GetInt("redis.single.db")),
		components.WithRedisPoolSize(conf.GetInt("redis.single.pool_size")),
		components.WithRedisMinIdleConns(conf.GetInt("redis.single.min_idle_conns")),
		components.WithRedisDialTimeout(conf.GetStringTimeDuration("redis.single.dial_timeout")),
		components.WithRedisReadTimeout(conf.GetStringTimeDuration("redis.single.read_timeout")),
		components.WithRedisWriteTimeout(conf.GetStringTimeDuration("redis.single.write_timeout")),
		components.WithRedisMaxRetries(conf.GetInt("redis.single.max_retries")),
	)
	f.RegisterComponent(redisComponent)
}

// SetupRedisCluster 初始化Redis集群组件
func SetupRedisCluster(f *frame.Frame, conf *config.ConfigComponent) {
	redisClusterComponent := components.NewRedisClusterComponent(
		components.WithClusterAddrs(conf.GetStringSlice("redis.cluster.nodes")),
		components.WithClusterPassword(conf.GetString("redis.cluster.password")),
		components.WithClusterPoolSize(conf.GetInt("redis.cluster.pool_size")),
		components.WithClusterMinIdleConns(conf.GetInt("redis.cluster.min_idle_conns")),
		components.WithClusterDialTimeout(conf.GetStringTimeDuration("redis.cluster.dial_timeout")),
		components.WithClusterReadTimeout(conf.GetStringTimeDuration("redis.cluster.read_timeout")),
		components.WithClusterWriteTimeout(conf.GetStringTimeDuration("redis.cluster.write_timeout")),
		components.WithClusterMaxRetries(conf.GetInt("redis.cluster.max_retries")),
		components.WithClusterRouteRandomly(conf.GetBool("redis.cluster.route_randomly")),
		components.WithClusterMinRetryBackoff(conf.GetStringTimeDuration("redis.cluster.min_retry_backoff")),
		components.WithClusterMaxRetryBackoff(conf.GetStringTimeDuration("redis.cluster.max_retry_backoff")),
	)
	f.RegisterComponent(redisClusterComponent)
}
