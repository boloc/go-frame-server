package cache

import (
	"context"
	"time"

	"github.com/boloc/go-frame-server/internal/example/constant"
	"github.com/boloc/go-frame-server/internal/example/repository"
	"github.com/boloc/go-frame-server/pkg/errs"
	"github.com/boloc/go-frame-server/pkg/frame/refreshcache"
)

// ProductActiveCount 缓存当前上架产品数量：30s 刷 Redis，5s 刷内存。
//
// Get 优先读内存。warmUp 失败、内存从未成功加载过时，Get 会同步调一次 Loader（singleflight
// 合并并发），但不写回缓存——等定时刷新成功后才进内存。两边都拿不到时 ok=false。
// Jitter 给刷新加随机抖动，避免多实例在同一调度边界一起打数据源/Redis。
// register 已经把它登记好，handler 直接 Get 即可。
var ProductActiveCount = register("产品上架数量缓存", refreshcache.Options[int64]{
	Key: constant.CacheKeyProductActiveCount,
	Loader: func(ctx context.Context) (int64, error) {
		return repository.NewProductRepository().CountActive(ctx)
	},
	RedisInterval:  30 * time.Second,
	MemoryInterval: 5 * time.Second,
	Jitter:         5 * time.Second,
})

// ProductNotice 缓存产品详情页的全局公告文案（读 config_db）：60s 刷 Redis，10s 刷内存。
// Get / Jitter 语义与 ProductActiveCount 相同。
var ProductNotice = register("产品详情页公告缓存", refreshcache.Options[string]{
	Key: constant.CacheKeyProductNotice,
	Loader: func(ctx context.Context) (string, error) {
		notice, err := repository.NewSystemConfigRepository().GetValue(ctx, constant.ConfigKeyProductNotice)
		if err != nil {
			if errs.Is(err, errs.CodeNotFound) {
				return "", nil // 没配置公告是正常状态，不是加载失败
			}
			return "", err
		}
		return notice, nil
	},
	RedisInterval:  60 * time.Second,
	MemoryInterval: 10 * time.Second,
	Jitter:         5 * time.Second,
})
