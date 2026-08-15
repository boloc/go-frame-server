package frame

import (
	"github.com/boloc/go-frame-server/pkg/frame/components"

	"github.com/redis/go-redis/v9"
)

// panic 版访问器适合启动阶段；运行时请用对应的 Try 版本。

// TryGetRedis 获取全局 Redis 单机实例；未初始化时返回 (nil, false)，不 panic。
func TryGetRedis() (*redis.Client, bool) {
	if components.GlobalRedisComponent == nil {
		return nil, false
	}
	return components.GlobalRedisComponent.GetClient(), true
}

// GetRedis 获取全局 Redis 单机实例；未初始化时 panic。运行时请用 TryGetRedis。
func GetRedis() *redis.Client {
	client, ok := TryGetRedis()
	if !ok {
		panic("redis component is not initialized")
	}
	return client
}

// TryGetRedisCluster 获取全局 Redis 集群实例；未初始化时返回 (nil, false)，不 panic。
func TryGetRedisCluster() (*redis.ClusterClient, bool) {
	if components.GlobalRedisClusterComponent == nil {
		return nil, false
	}
	return components.GlobalRedisClusterComponent.GetClient(), true
}

// GetRedisCluster 获取全局 Redis 集群实例；未初始化时 panic。运行时请用 TryGetRedisCluster。
func GetRedisCluster() *redis.ClusterClient {
	client, ok := TryGetRedisCluster()
	if !ok {
		panic("redis cluster component is not initialized")
	}
	return client
}

// TryGetRedisSentinel 获取全局 Redis 哨兵实例；未初始化时返回 (nil, false)，不 panic。
// 返回类型与 GetRedis 相同，均为 *redis.Client。
func TryGetRedisSentinel() (*redis.Client, bool) {
	if components.GlobalRedisSentinelComponent == nil {
		return nil, false
	}
	return components.GlobalRedisSentinelComponent.GetClient(), true
}

// GetRedisSentinel 获取全局 Redis 哨兵实例；未初始化时 panic。运行时请用 TryGetRedisSentinel。
func GetRedisSentinel() *redis.Client {
	client, ok := TryGetRedisSentinel()
	if !ok {
		panic("redis sentinel component is not initialized")
	}
	return client
}

// TryGetRedisCmdable 获取通用 Redis 接口，按 集群 > 哨兵 > 单机 适配；均未初始化时返回 (nil, false)。
func TryGetRedisCmdable() (redis.Cmdable, bool) {
	if components.GlobalRedisClusterComponent != nil {
		return components.GlobalRedisClusterComponent.GetClient(), true
	}
	if components.GlobalRedisSentinelComponent != nil {
		return components.GlobalRedisSentinelComponent.GetClient(), true
	}
	if components.GlobalRedisComponent != nil {
		return components.GlobalRedisComponent.GetClient(), true
	}
	return nil, false
}

// GetRedisCmdable 获取通用 Redis 接口（集群 > 哨兵 > 单机）；均未初始化时 panic。运行时请用 TryGetRedisCmdable。
func GetRedisCmdable() redis.Cmdable {
	client, ok := TryGetRedisCmdable()
	if !ok {
		panic("redis is not initialized (neither single, sentinel, nor cluster)")
	}
	return client
}
