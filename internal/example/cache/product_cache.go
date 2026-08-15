// Package cache 存放本示例的双层刷新只读缓存实例。
package cache

import (
	"context"
	"time"

	"github.com/boloc/go-frame-server/internal/example/repository"
	"github.com/boloc/go-frame-server/pkg/frame/refreshcache"
)

// ProductActiveCount 缓存当前上架产品数量：30s 刷 Redis，5s 刷内存。
// 使用前需在 bootstrap 里注册，handler 直接 Get 即可。
var ProductActiveCount = refreshcache.New(refreshcache.Options[int64]{
	Key: "example:product:active_count",
	Loader: func(ctx context.Context) (int64, error) {
		return repository.NewProductRepository().CountActive(ctx)
	},
	RedisInterval:  30 * time.Second,
	MemoryInterval: 5 * time.Second,
	Jitter:         5 * time.Second,
})
