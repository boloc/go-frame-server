package components

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// 全局Redis组件
var GlobalRedisComponent *RedisComponent

// RedisOption 定义Redis选项函数类型
type RedisOption func(*RedisComponent)

// RedisComponent Redis组件
type RedisComponent struct {
	client *redis.Client
	config *redis.Options
}

// WithRedisAddr 设置Redis地址
func WithRedisAddr(addr string) RedisOption {
	return func(r *RedisComponent) {
		r.config.Addr = addr
	}
}

// WithRedisPassword 设置Redis密码
func WithRedisPassword(password string) RedisOption {
	return func(r *RedisComponent) {
		r.config.Password = password
	}
}

// WithRedisDB 设置Redis数据库
func WithRedisDB(db int) RedisOption {
	return func(r *RedisComponent) {
		r.config.DB = db
	}
}

// WithRedisPoolSize 设置连接池大小
func WithRedisPoolSize(poolSize int) RedisOption {
	return func(r *RedisComponent) {
		r.config.PoolSize = poolSize
	}
}

// WithRedisMinIdleConns 设置最小空闲连接数
func WithRedisMinIdleConns(minIdleConns int) RedisOption {
	return func(r *RedisComponent) {
		r.config.MinIdleConns = minIdleConns
	}
}

// WithRedisReadTimeout 设置读取超时时间
func WithRedisReadTimeout(readTimeout time.Duration) RedisOption {
	return func(r *RedisComponent) {
		r.config.ReadTimeout = readTimeout
	}
}

// WithRedisWriteTimeout 设置写入超时时间
func WithRedisWriteTimeout(writeTimeout time.Duration) RedisOption {
	return func(r *RedisComponent) {
		r.config.WriteTimeout = writeTimeout
	}
}

// WithRedisMaxRetries 设置最大重试次数
func WithRedisMaxRetries(maxRetries int) RedisOption {
	return func(r *RedisComponent) {
		r.config.MaxRetries = maxRetries
	}
}

// WithRedisPoolTimeout 设置连接池超时时间
func WithRedisPoolTimeout(poolTimeout time.Duration) RedisOption {
	return func(r *RedisComponent) {
		r.config.PoolTimeout = poolTimeout
	}
}

// WithRedisMinRetryBackoff 设置最小重试间隔时间
func WithRedisMinRetryBackoff(minRetryBackoff time.Duration) RedisOption {
	return func(r *RedisComponent) {
		r.config.MinRetryBackoff = minRetryBackoff
	}
}

// WithRedisMaxRetryBackoff 设置最大重试间隔时间
func WithRedisMaxRetryBackoff(maxRetryBackoff time.Duration) RedisOption {
	return func(r *RedisComponent) {
		r.config.MaxRetryBackoff = maxRetryBackoff
	}
}

// NewRedisComponent 创建Redis组件
func NewRedisComponent(opts ...RedisOption) *RedisComponent {
	r := &RedisComponent{
		config: &redis.Options{
			Addr:         "localhost:6379", // 地址
			DB:           0,                // 数据库
			PoolSize:     10,               // 连接池大小
			MinIdleConns: 10,               // 最小空闲连接数
			ReadTimeout:  5 * time.Second,  // 读取超时时间
			WriteTimeout: 5 * time.Second,  // 写入超时时间
			MaxRetries:   3,                // 最大重试次数
			PoolTimeout:  5 * time.Second,  // 连接池超时时间
		},
	}
	for _, opt := range opts {
		opt(r)
	}
	GlobalRedisComponent = r
	return r
}

// Start 启动Redis组件
func (r *RedisComponent) Start(ctx context.Context) error {
	r.client = redis.NewClient(r.config)
	if err := r.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("failed to connect to redis: %v", err)
	}
	return nil
}

// Stop 停止Redis组件
func (r *RedisComponent) Stop(ctx context.Context) error {
	if r.client != nil {
		return r.client.Close()
	}
	return nil
}

// GetClient 获取Redis客户端
func (r *RedisComponent) GetClient() *redis.Client {
	return r.client
}
