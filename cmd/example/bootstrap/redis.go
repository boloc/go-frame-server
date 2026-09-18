package bootstrap

import (
	"time"

	"github.com/boloc/go-frame-server/v2/pkg/frame"
	"github.com/boloc/go-frame-server/v2/pkg/frame/components"
	"github.com/boloc/go-frame-server/v2/pkg/frame/config"
	"github.com/boloc/go-frame-server/v2/pkg/frame/rediskey"
)

/*
	三种模式只注册一个。业务侧一律 frame.GetRedisCmdable / TryGetRedisCmdable。
	连接池未配时框架默认 PoolSize=32 / MinIdleConns=4；route_randomly 保持 false，
	避免幂等/限流/Exclusive 锁读到从节点滞后数据。
*/

// setupRedisNamespace 把 server.name 加到本进程所有 Redis key 前面。
// 例如 name=frame-example 时，缓存 key 会写成 frame-example:cache:...，
// 这样和别的应用写到同一个 Redis 里的 key 能分开。
func setupRedisNamespace(conf *config.ConfigComponent) {
	var cfg ServerConfig
	conf.MustStrictUnmarshalKey("server", &cfg)
	rediskey.SetNamespace(cfg.Name)
}

// RedisSingleConfig 对应配置段 redis.single。
type RedisSingleConfig struct {
	Addr         string        `mapstructure:"addr" validate:"required"`
	Password     string        `mapstructure:"password"`
	DB           int           `mapstructure:"db"`
	PoolSize     int           `mapstructure:"pool_size" validate:"min=0"`
	MinIdleConns int           `mapstructure:"min_idle_conns" validate:"min=0"`
	DialTimeout  time.Duration `mapstructure:"dial_timeout"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
	MaxRetries   int           `mapstructure:"max_retries" validate:"min=0"`
}

// RedisClusterConfig 对应配置段 redis.cluster。
type RedisClusterConfig struct {
	Nodes           []string      `mapstructure:"nodes" validate:"required"`
	Password        string        `mapstructure:"password"`
	PoolSize        int           `mapstructure:"pool_size" validate:"min=0"`
	MinIdleConns    int           `mapstructure:"min_idle_conns" validate:"min=0"`
	DialTimeout     time.Duration `mapstructure:"dial_timeout"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`
	MaxRetries      int           `mapstructure:"max_retries" validate:"min=0"`
	RouteRandomly   bool          `mapstructure:"route_randomly"`
	MinRetryBackoff time.Duration `mapstructure:"min_retry_backoff"`
	MaxRetryBackoff time.Duration `mapstructure:"max_retry_backoff"`
}

// RedisSentinelConfig 对应配置段 redis.sentinel。
type RedisSentinelConfig struct {
	MasterName   string        `mapstructure:"master_name" validate:"required"`
	Addrs        []string      `mapstructure:"addrs" validate:"required"`
	Password     string        `mapstructure:"password"`
	DataPassword string        `mapstructure:"data_password"`
	DB           int           `mapstructure:"db"`
	PoolSize     int           `mapstructure:"pool_size" validate:"min=0"`
	MinIdleConns int           `mapstructure:"min_idle_conns" validate:"min=0"`
	DialTimeout  time.Duration `mapstructure:"dial_timeout"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
	MaxRetries   int           `mapstructure:"max_retries" validate:"min=0"`
}

// SetupRedis 初始化Redis单机组件，返回创建的组件实例方便调用方（如 SetupMonitor）
// 挂上连接池指标采集器。
func SetupRedis(f *frame.Frame, conf *config.ConfigComponent) *components.RedisComponent {
	setupRedisNamespace(conf)

	var cfg RedisSingleConfig
	conf.MustStrictUnmarshalKey("redis.single", &cfg)

	redisComponent := components.NewRedisComponent(
		components.WithRedisAddr(cfg.Addr),
		components.WithRedisPassword(cfg.Password),
		components.WithRedisDB(cfg.DB),
		components.WithRedisPoolSize(cfg.PoolSize),
		components.WithRedisMinIdleConns(cfg.MinIdleConns),
		components.WithRedisDialTimeout(cfg.DialTimeout),
		components.WithRedisReadTimeout(cfg.ReadTimeout),
		components.WithRedisWriteTimeout(cfg.WriteTimeout),
		components.WithRedisMaxRetries(cfg.MaxRetries),
	)
	f.RegisterComponent(redisComponent)
	return redisComponent
}

// SetupRedisCluster 初始化 Redis 集群组件，返回值给 SetupMonitor 挂连接池指标。
func SetupRedisCluster(f *frame.Frame, conf *config.ConfigComponent) *components.RedisClusterComponent {
	setupRedisNamespace(conf)

	var cfg RedisClusterConfig
	conf.MustStrictUnmarshalKey("redis.cluster", &cfg)

	redisClusterComponent := components.NewRedisClusterComponent(
		components.WithClusterAddrs(cfg.Nodes),
		components.WithClusterPassword(cfg.Password),
		components.WithClusterPoolSize(cfg.PoolSize),
		components.WithClusterMinIdleConns(cfg.MinIdleConns),
		components.WithClusterDialTimeout(cfg.DialTimeout),
		components.WithClusterReadTimeout(cfg.ReadTimeout),
		components.WithClusterWriteTimeout(cfg.WriteTimeout),
		components.WithClusterMaxRetries(cfg.MaxRetries),
		components.WithClusterRouteRandomly(cfg.RouteRandomly),
		components.WithClusterMinRetryBackoff(cfg.MinRetryBackoff),
		components.WithClusterMaxRetryBackoff(cfg.MaxRetryBackoff),
	)
	f.RegisterComponent(redisClusterComponent)
	return redisClusterComponent
}

// SetupRedisSentinel 初始化 Redis 哨兵组件，返回值给 SetupMonitor 挂连接池指标。
func SetupRedisSentinel(f *frame.Frame, conf *config.ConfigComponent) *components.RedisSentinelComponent {
	setupRedisNamespace(conf)

	var cfg RedisSentinelConfig
	conf.MustStrictUnmarshalKey("redis.sentinel", &cfg)

	redisSentinelComponent := components.NewRedisSentinelComponent(
		components.WithSentinelMasterName(cfg.MasterName),
		components.WithSentinelAddrs(cfg.Addrs),
		components.WithSentinelPassword(cfg.Password),
		components.WithSentinelDataPassword(cfg.DataPassword),
		components.WithSentinelDB(cfg.DB),
		components.WithSentinelPoolSize(cfg.PoolSize),
		components.WithSentinelMinIdleConns(cfg.MinIdleConns),
		components.WithSentinelDialTimeout(cfg.DialTimeout),
		components.WithSentinelReadTimeout(cfg.ReadTimeout),
		components.WithSentinelWriteTimeout(cfg.WriteTimeout),
		components.WithSentinelMaxRetries(cfg.MaxRetries),
	)
	f.RegisterComponent(redisSentinelComponent)
	return redisSentinelComponent
}
