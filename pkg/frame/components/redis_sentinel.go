// Redis Sentinel 组件：客户端连接哨兵节点，由哨兵告知当前主库；故障转移对业务透明。
package components

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// GlobalRedisSentinelComponent 全局 Redis 哨兵组件，仅在 Start 的 Ping 成功后赋值，Stop 后清空。
var GlobalRedisSentinelComponent *RedisSentinelComponent

// RedisSentinelOption 定义Redis哨兵选项函数类型
type RedisSentinelOption func(*RedisSentinelComponent)

// RedisSentinelComponent Redis哨兵组件
type RedisSentinelComponent struct {
	mu     sync.RWMutex
	client *redis.Client
	config *redis.FailoverOptions

	// connectRetryAttempts/connectRetryInterval 控制 Start 初次连接的重试，默认 3 次、间隔 2s。
	connectRetryAttempts int
	connectRetryInterval time.Duration
}

// WithSentinelMasterName 设置哨兵监控的主库名称（哨兵配置文件里 `sentinel monitor <name> ...` 的 name）。
func WithSentinelMasterName(name string) RedisSentinelOption {
	return func(r *RedisSentinelComponent) {
		r.config.MasterName = name
	}
}

// WithSentinelAddrs 设置哨兵节点地址列表（不是 Redis 数据节点地址）。
func WithSentinelAddrs(addrs []string) RedisSentinelOption {
	return func(r *RedisSentinelComponent) {
		r.config.SentinelAddrs = addrs
	}
}

// WithSentinelPassword 设置哨兵节点密码（与数据节点密码不同，见 WithSentinelDataPassword）。
func WithSentinelPassword(password string) RedisSentinelOption {
	return func(r *RedisSentinelComponent) {
		r.config.SentinelPassword = password
	}
}

// WithSentinelDataPassword 设置连接 Redis 数据节点（主/从）的密码。
func WithSentinelDataPassword(password string) RedisSentinelOption {
	return func(r *RedisSentinelComponent) {
		r.config.Password = password
	}
}

// WithSentinelDB 设置数据库编号。
func WithSentinelDB(db int) RedisSentinelOption {
	return func(r *RedisSentinelComponent) {
		r.config.DB = db
	}
}

// WithSentinelPoolSize 设置连接池大小。
func WithSentinelPoolSize(poolSize int) RedisSentinelOption {
	return func(r *RedisSentinelComponent) {
		r.config.PoolSize = poolSize
	}
}

// WithSentinelMinIdleConns 设置最小空闲连接数。
func WithSentinelMinIdleConns(minIdleConns int) RedisSentinelOption {
	return func(r *RedisSentinelComponent) {
		r.config.MinIdleConns = minIdleConns
	}
}

// WithSentinelConnMaxIdleTime 设置连接空闲超过这个时间就剔除重建。
func WithSentinelConnMaxIdleTime(d time.Duration) RedisSentinelOption {
	return func(r *RedisSentinelComponent) {
		r.config.ConnMaxIdleTime = d
	}
}

// WithSentinelDialTimeout 设置连接超时时间。
func WithSentinelDialTimeout(dialTimeout time.Duration) RedisSentinelOption {
	return func(r *RedisSentinelComponent) {
		r.config.DialTimeout = dialTimeout
	}
}

// WithSentinelReadTimeout 设置读取超时时间。
func WithSentinelReadTimeout(readTimeout time.Duration) RedisSentinelOption {
	return func(r *RedisSentinelComponent) {
		r.config.ReadTimeout = readTimeout
	}
}

// WithSentinelWriteTimeout 设置写入超时时间。
func WithSentinelWriteTimeout(writeTimeout time.Duration) RedisSentinelOption {
	return func(r *RedisSentinelComponent) {
		r.config.WriteTimeout = writeTimeout
	}
}

// WithSentinelMaxRetries 设置最大重试次数。
func WithSentinelMaxRetries(maxRetries int) RedisSentinelOption {
	return func(r *RedisSentinelComponent) {
		r.config.MaxRetries = maxRetries
	}
}

// WithSentinelReplicaOnly 为 true 时所有命令走只读从节点。默认 false，读写都走主节点。
func WithSentinelReplicaOnly(replicaOnly bool) RedisSentinelOption {
	return func(r *RedisSentinelComponent) {
		r.config.ReplicaOnly = replicaOnly
	}
}

// WithSentinelConnectRetryAttempts 设置 Start() 阶段初次连接的重试次数（含第一次尝试），默认 3。
func WithSentinelConnectRetryAttempts(attempts int) RedisSentinelOption {
	return func(r *RedisSentinelComponent) {
		r.connectRetryAttempts = attempts
	}
}

// WithSentinelConnectRetryInterval 设置 Start() 阶段初次连接每次重试之间的等待时间，默认 2s。
func WithSentinelConnectRetryInterval(interval time.Duration) RedisSentinelOption {
	return func(r *RedisSentinelComponent) {
		r.connectRetryInterval = interval
	}
}

// NewRedisSentinelComponent 创建Redis哨兵组件。
func NewRedisSentinelComponent(opts ...RedisSentinelOption) *RedisSentinelComponent {
	r := &RedisSentinelComponent{
		config: &redis.FailoverOptions{
			PoolSize:     10,
			MinIdleConns: 10,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 5 * time.Second,
			MaxRetries:   3,
		},
		connectRetryAttempts: 3,
		connectRetryInterval: 2 * time.Second,
	}

	for _, opt := range opts {
		opt(r)
	}

	return r
}

// Start 启动 Redis 哨兵组件：Ping 成功后发布为全局实例。初次连接失败会按 connectRetry 重试。
func (r *RedisSentinelComponent) Start(ctx context.Context) error {
	if r.config.MasterName == "" {
		return fmt.Errorf("redis sentinel: MasterName 未配置（对应哨兵 monitor 的 name）")
	}
	if len(r.config.SentinelAddrs) == 0 {
		return fmt.Errorf("redis sentinel: SentinelAddrs 未配置")
	}

	var client *redis.Client
	err := retryConnect(ctx, r.connectRetryAttempts, r.connectRetryInterval, func() error {
		c := redis.NewFailoverClient(r.config)
		if pingErr := c.Ping(ctx).Err(); pingErr != nil {
			_ = c.Close()
			return pingErr
		}
		client = c
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to connect to redis via sentinel: %w", err)
	}

	r.mu.Lock()
	r.client = client
	r.mu.Unlock()

	GlobalRedisSentinelComponent = r
	return nil
}

// Stop 停止Redis哨兵组件
func (r *RedisSentinelComponent) Stop(ctx context.Context) error {
	r.mu.Lock()
	client := r.client
	r.client = nil
	r.mu.Unlock()

	if GlobalRedisSentinelComponent == r {
		GlobalRedisSentinelComponent = nil
	}

	if client == nil {
		return nil
	}
	return client.Close()
}

// GetClient 获取 Redis 客户端（类型与单机模式相同，均为 *redis.Client）。
func (r *RedisSentinelComponent) GetClient() *redis.Client {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.client
}

// PoolStats 返回连接池状态；未 Start 或已 Stop 时返回 nil。
func (r *RedisSentinelComponent) PoolStats() *redis.PoolStats {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.client == nil {
		return nil
	}
	return r.client.PoolStats()
}
