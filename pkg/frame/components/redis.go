package components

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// GlobalRedisComponent 全局 Redis 组件，仅在 Start 的 Ping 成功后赋值，Stop 后清空。
var GlobalRedisComponent *RedisComponent

// RedisOption 定义Redis选项函数类型
type RedisOption func(*RedisComponent)

// RedisComponent Redis组件
type RedisComponent struct {
	mu     sync.RWMutex
	client *redis.Client
	config *redis.Options

	// connectRetryAttempts/connectRetryInterval 控制 Start 初次连接的重试，默认 3 次、间隔 2s。
	// 与 go-redis 的 MaxRetries（命令级重试）互不影响。
	connectRetryAttempts int
	connectRetryInterval time.Duration
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

// WithRedisConnMaxIdleTime 设置连接空闲超时。默认沿用 go-redis（30 分钟）。
func WithRedisConnMaxIdleTime(d time.Duration) RedisOption {
	return func(r *RedisComponent) {
		r.config.ConnMaxIdleTime = d
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

// WithRedisDialTimeout 设置连接超时时间
func WithRedisDialTimeout(dialTimeout time.Duration) RedisOption {
	return func(r *RedisComponent) {
		r.config.DialTimeout = dialTimeout
	}
}

// WithRedisConnectRetryAttempts 设置 Start() 阶段初次连接的重试次数（含第一次尝试），默认 3。
func WithRedisConnectRetryAttempts(attempts int) RedisOption {
	return func(r *RedisComponent) {
		r.connectRetryAttempts = attempts
	}
}

// WithRedisConnectRetryInterval 设置 Start() 阶段初次连接每次重试之间的等待时间，默认 2s。
func WithRedisConnectRetryInterval(interval time.Duration) RedisOption {
	return func(r *RedisComponent) {
		r.connectRetryInterval = interval
	}
}

// NewRedisComponent 创建单机 Redis 组件。
// 不做主从发现或读写分离；需要故障转移请用 Sentinel，或让 Addr 指向 VIP/代理。
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
		connectRetryAttempts: 3,
		connectRetryInterval: 2 * time.Second,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Start 启动 Redis 组件：Ping 成功后发布为全局实例。初次连接失败会按 connectRetry 重试。
func (r *RedisComponent) Start(ctx context.Context) error {
	var client *redis.Client
	err := retryConnect(ctx, r.connectRetryAttempts, r.connectRetryInterval, func() error {
		c := redis.NewClient(r.config)
		if pingErr := c.Ping(ctx).Err(); pingErr != nil {
			_ = c.Close() // Ping 失败时主动关闭，避免半初始化的连接泄漏
			return pingErr
		}
		client = c
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to connect to redis: %w", err)
	}

	r.mu.Lock()
	r.client = client
	r.mu.Unlock()

	GlobalRedisComponent = r
	return nil
}

// Stop 停止Redis组件
func (r *RedisComponent) Stop(ctx context.Context) error {
	r.mu.Lock()
	client := r.client
	r.client = nil
	r.mu.Unlock()

	if GlobalRedisComponent == r {
		GlobalRedisComponent = nil
	}

	if client == nil {
		return nil
	}
	return client.Close()
}

// GetClient 获取Redis客户端
func (r *RedisComponent) GetClient() *redis.Client {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.client
}

// PoolStats 返回连接池状态；未 Start 或已 Stop 时返回 nil。
func (r *RedisComponent) PoolStats() *redis.PoolStats {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.client == nil {
		return nil
	}
	return r.client.PoolStats()
}
