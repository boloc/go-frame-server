package bootstrap

import (
	"github.com/boloc/go-frame-server/pkg/frame"
	"github.com/boloc/go-frame-server/pkg/frame/components"
	"github.com/boloc/go-frame-server/pkg/frame/config"
)

// SetupRedis 初始化Redis单机组件，返回创建的组件实例方便调用方（如 SetupMonitor）
// 挂上连接池指标采集器。
func SetupRedis(f *frame.Frame, conf *config.ConfigComponent) *components.RedisComponent {
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
	return redisComponent
}

// SetupRedisCluster 初始化 Redis 集群组件，返回值给 SetupMonitor 挂连接池指标。
func SetupRedisCluster(f *frame.Frame, conf *config.ConfigComponent) *components.RedisClusterComponent {
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
	return redisClusterComponent
}

// SetupRedisSentinel 初始化 Redis 哨兵组件，返回值给 SetupMonitor 挂连接池指标。
func SetupRedisSentinel(f *frame.Frame, conf *config.ConfigComponent) *components.RedisSentinelComponent {
	redisSentinelComponent := components.NewRedisSentinelComponent(
		components.WithSentinelMasterName(conf.GetString("redis.sentinel.master_name")),
		components.WithSentinelAddrs(conf.GetStringSlice("redis.sentinel.addrs")),
		components.WithSentinelPassword(conf.GetString("redis.sentinel.password")),
		components.WithSentinelDataPassword(conf.GetString("redis.sentinel.data_password")),
		components.WithSentinelDB(conf.GetInt("redis.sentinel.db")),
		components.WithSentinelPoolSize(conf.GetInt("redis.sentinel.pool_size")),
		components.WithSentinelMinIdleConns(conf.GetInt("redis.sentinel.min_idle_conns")),
		components.WithSentinelDialTimeout(conf.GetStringTimeDuration("redis.sentinel.dial_timeout")),
		components.WithSentinelReadTimeout(conf.GetStringTimeDuration("redis.sentinel.read_timeout")),
		components.WithSentinelWriteTimeout(conf.GetStringTimeDuration("redis.sentinel.write_timeout")),
		components.WithSentinelMaxRetries(conf.GetInt("redis.sentinel.max_retries")),
	)
	f.RegisterComponent(redisSentinelComponent)
	return redisSentinelComponent
}
