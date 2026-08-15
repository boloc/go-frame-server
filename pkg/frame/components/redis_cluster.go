package components

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// GlobalRedisClusterComponent 全局 Redis 集群组件，仅在 Start 的 Ping 成功后赋值，Stop 后清空。
var GlobalRedisClusterComponent *RedisClusterComponent

// RedisClusterOption 定义Redis集群选项函数类型
type RedisClusterOption func(*RedisClusterComponent)

// RedisClusterComponent Redis集群组件
type RedisClusterComponent struct {
	mu     sync.RWMutex
	client *redis.ClusterClient
	config *redis.ClusterOptions

	// connectRetryAttempts/connectRetryInterval 控制 Start 初次连接的重试，默认 3 次、间隔 2s。
	connectRetryAttempts int
	connectRetryInterval time.Duration
}

// WithClusterAddrs 设置Redis集群地址
func WithClusterAddrs(addrs []string) RedisClusterOption {
	return func(r *RedisClusterComponent) {
		r.config.Addrs = addrs
	}
}

// WithClusterPassword 设置Redis集群密码
func WithClusterPassword(password string) RedisClusterOption {
	return func(r *RedisClusterComponent) {
		r.config.Password = password
	}
}

// WithClusterPoolSize 设置集群连接池大小
func WithClusterPoolSize(poolSize int) RedisClusterOption {
	return func(r *RedisClusterComponent) {
		r.config.PoolSize = poolSize
	}
}

// WithClusterMinIdleConns 设置集群最小空闲连接数
func WithClusterMinIdleConns(minIdleConns int) RedisClusterOption {
	return func(r *RedisClusterComponent) {
		r.config.MinIdleConns = minIdleConns
	}
}

// WithClusterConnMaxIdleTime 设置集群连接空闲超过这个时间就剔除重建。
func WithClusterConnMaxIdleTime(d time.Duration) RedisClusterOption {
	return func(r *RedisClusterComponent) {
		r.config.ConnMaxIdleTime = d
	}
}

// WithClusterDialTimeout 设置集群连接超时时间
func WithClusterDialTimeout(dialTimeout time.Duration) RedisClusterOption {
	return func(r *RedisClusterComponent) {
		r.config.DialTimeout = dialTimeout
	}
}

// WithClusterRouteRandomly 设置集群是否随机路由
func WithClusterRouteRandomly(routeRandomly bool) RedisClusterOption {
	return func(r *RedisClusterComponent) {
		r.config.RouteRandomly = routeRandomly
	}
}

// WithClusterMaxRetries 设置集群最大重试次数
func WithClusterMaxRetries(maxRetries int) RedisClusterOption {
	return func(r *RedisClusterComponent) {
		r.config.MaxRetries = maxRetries
	}
}

// WithClusterReadTimeout 设置集群读取超时时间
func WithClusterReadTimeout(readTimeout time.Duration) RedisClusterOption {
	return func(r *RedisClusterComponent) {
		r.config.ReadTimeout = readTimeout
	}
}

// WithClusterWriteTimeout 设置集群写入超时时间
func WithClusterWriteTimeout(writeTimeout time.Duration) RedisClusterOption {
	return func(r *RedisClusterComponent) {
		r.config.WriteTimeout = writeTimeout
	}
}

// WithClusterPoolTimeout 设置集群连接池超时时间
func WithClusterPoolTimeout(poolTimeout time.Duration) RedisClusterOption {
	return func(r *RedisClusterComponent) {
		r.config.PoolTimeout = poolTimeout
	}
}

// WithClusterMinRetryBackoff 设置集群最小重试间隔时间
func WithClusterMinRetryBackoff(minRetryBackoff time.Duration) RedisClusterOption {
	return func(r *RedisClusterComponent) {
		r.config.MinRetryBackoff = minRetryBackoff
	}
}

// WithClusterMaxRetryBackoff 设置集群最大重试间隔时间
func WithClusterMaxRetryBackoff(maxRetryBackoff time.Duration) RedisClusterOption {
	return func(r *RedisClusterComponent) {
		r.config.MaxRetryBackoff = maxRetryBackoff
	}
}

// WithClusterConnectRetryAttempts 设置 Start() 阶段初次连接的重试次数（含第一次尝试），默认 3。
func WithClusterConnectRetryAttempts(attempts int) RedisClusterOption {
	return func(r *RedisClusterComponent) {
		r.connectRetryAttempts = attempts
	}
}

// WithClusterConnectRetryInterval 设置 Start() 阶段初次连接每次重试之间的等待时间，默认 2s。
func WithClusterConnectRetryInterval(interval time.Duration) RedisClusterOption {
	return func(r *RedisClusterComponent) {
		r.connectRetryInterval = interval
	}
}

// NewRedisClusterComponent 创建 Redis 集群组件。
// 分片故障转移由 Cluster 协议和 go-redis 处理，应用无需介入。
func NewRedisClusterComponent(opts ...RedisClusterOption) *RedisClusterComponent {
	r := &RedisClusterComponent{
		config: &redis.ClusterOptions{
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 5 * time.Second,
			PoolSize:     10,
			MinIdleConns: 10,
			MaxRetries:   3,
			PoolTimeout:  5 * time.Second,
		},
		connectRetryAttempts: 3,
		connectRetryInterval: 2 * time.Second,
	}

	for _, opt := range opts {
		opt(r)
	}

	return r
}

// Start 启动 Redis 集群组件：Ping 成功后发布为全局实例。初次连接失败会按 connectRetry 重试。
func (r *RedisClusterComponent) Start(ctx context.Context) error {
	var client *redis.ClusterClient
	err := retryConnect(ctx, r.connectRetryAttempts, r.connectRetryInterval, func() error {
		c := redis.NewClusterClient(r.config)
		if pingErr := c.Ping(ctx).Err(); pingErr != nil {
			_ = c.Close()
			return pingErr
		}
		client = c
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to connect to redis cluster: %w", err)
	}

	r.mu.Lock()
	r.client = client
	r.mu.Unlock()

	GlobalRedisClusterComponent = r
	return nil
}

// Stop 停止Redis集群组件
func (r *RedisClusterComponent) Stop(ctx context.Context) error {
	r.mu.Lock()
	client := r.client
	r.client = nil
	r.mu.Unlock()

	if GlobalRedisClusterComponent == r {
		GlobalRedisClusterComponent = nil
	}

	if client == nil {
		return nil
	}
	return client.Close()
}

// GetClient 获取Redis集群客户端
func (r *RedisClusterComponent) GetClient() *redis.ClusterClient {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.client
}

// PoolStats 返回连接池状态；未 Start 或已 Stop 时返回 nil。
func (r *RedisClusterComponent) PoolStats() *redis.PoolStats {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.client == nil {
		return nil
	}
	return r.client.PoolStats()
}
