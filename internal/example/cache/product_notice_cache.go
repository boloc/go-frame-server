package cache

import (
	"context"
	"time"

	"github.com/boloc/go-frame-server/internal/example/constant"
	"github.com/boloc/go-frame-server/internal/example/repository"
	"github.com/boloc/go-frame-server/pkg/errs"
	"github.com/boloc/go-frame-server/pkg/frame/refreshcache"
)

// ProductNotice 缓存产品详情页的全局公告文案（读 config_db）：60s 刷 Redis，10s 刷内存。
var ProductNotice = refreshcache.New(refreshcache.Options[string]{
	Key: "example:product:notice",
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
