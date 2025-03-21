package frame

import (
	"frame-server/pkg/frame/components"

	"github.com/redis/go-redis/v9"
)

// GetRedis 获取全局Redis单机实例
func GetRedis() *redis.Client {
	if components.GlobalRedisComponent == nil {
		panic("redis component is not initialized")
	}
	return components.GlobalRedisComponent.GetClient()
}

// GetRedisCluster 获取全局Redis集群实例
func GetRedisCluster() *redis.ClusterClient {
	if components.GlobalRedisClusterComponent == nil {
		panic("redis cluster component is not initialized")
	}
	return components.GlobalRedisClusterComponent.GetClient()
}
