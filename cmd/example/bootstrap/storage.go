package bootstrap

import (
	"context"

	"github.com/boloc/go-frame-server/pkg/frame/config"
	"github.com/boloc/go-frame-server/pkg/frame/storage"
	"github.com/boloc/go-frame-server/pkg/logger"
)

// SetupStorage 按 storage.r2 配置创建默认的对象存储客户端并注册为进程内默认实例
// （storage.SetDefault），供 handler 通过 storage.TryDefault 取用。对象存储是可选能力，
// 没配置 storage.r2.account_id 时直接跳过，不影响其它组件启动——不像 MySQL/Redis
// 那种"缺了就应该让进程直接起不来"的必需依赖。
func SetupStorage(conf *config.ConfigComponent) {
	accountID := conf.GetString("storage.r2.account_id")
	if accountID == "" {
		logger.Info("storage: 未配置 storage.r2.account_id，跳过对象存储初始化")
		return
	}

	client, err := storage.NewR2Client(context.Background(), storage.R2Config{
		AccountID:       accountID,
		AccessKeyID:     conf.GetString("storage.r2.access_key_id"),
		AccessKeySecret: conf.GetString("storage.r2.access_key_secret"),
		BucketName:      conf.GetString("storage.r2.bucket_name"),
		PublicBaseURL:   conf.GetString("storage.r2.public_base_url"),
	})
	if err != nil {
		panic("bootstrap: 初始化对象存储失败: " + err.Error())
	}
	storage.SetDefault(client)
}
