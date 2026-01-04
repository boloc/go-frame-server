package frame

import (
	"github.com/boloc/go-frame-server/pkg/frame/components"

	"github.com/redis/go-redis/v9"
)

// 获取全局Redis单机实例
func GetRedis() *redis.Client {
	if components.GlobalRedisComponent == nil {
		panic("redis component is not initialized")
	}
	return components.GlobalRedisComponent.GetClient()
}

// 获取全局Redis集群实例
func GetRedisCluster() *redis.ClusterClient {
	if components.GlobalRedisClusterComponent == nil {
		panic("redis cluster component is not initialized")
	}
	return components.GlobalRedisClusterComponent.GetClient()
}

//	获取通用Redis接口（自动适配单机/集群）
//
// 优先返回集群实例，其次返回单机实例
// 业务代码使用此方法无需关心底层是单机还是集群
func GetRedisCmdable() redis.Cmdable {
	if components.GlobalRedisClusterComponent != nil {
		return components.GlobalRedisClusterComponent.GetClient()
	}
	if components.GlobalRedisComponent != nil {
		return components.GlobalRedisComponent.GetClient()
	}
	panic("redis is not initialized (neither single nor cluster)")
}
