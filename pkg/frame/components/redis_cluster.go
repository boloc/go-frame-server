package components

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// 全局Redis集群组件
var GlobalRedisClusterComponent *RedisClusterComponent

// RedisClusterOption 定义Redis集群选项函数类型
type RedisClusterOption func(*RedisClusterComponent)

// RedisClusterComponent Redis集群组件
type RedisClusterComponent struct {
	client *redis.ClusterClient
	config *redis.ClusterOptions
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

// WithClusterTimeout 设置集群连接超时时间
func WithClusterTimeout(timeout time.Duration) RedisClusterOption {
	return func(r *RedisClusterComponent) {
		r.config.DialTimeout = timeout
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

// NewRedisClusterComponent 创建Redis集群组件
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
	}

	for _, opt := range opts {
		opt(r)
	}

	GlobalRedisClusterComponent = r
	return r
}

// Start 启动Redis集群组件
func (r *RedisClusterComponent) Start(ctx context.Context) error {
	r.client = redis.NewClusterClient(r.config)
	if err := r.client.Ping(ctx).Err(); err != nil {
		fmt.Println("打印错误ctx", ctx)
		return fmt.Errorf("failed to connect to redis cluster: %v", err)
	}
	return nil
}

// Stop 停止Redis集群组件
func (r *RedisClusterComponent) Stop(ctx context.Context) error {
	if r.client != nil {
		return r.client.Close()
	}
	return nil
}

// GetClient 获取Redis集群客户端
func (r *RedisClusterComponent) GetClient() *redis.ClusterClient {
	return r.client
}
