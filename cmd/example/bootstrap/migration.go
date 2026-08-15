package bootstrap

import (
	"context"

	"github.com/boloc/go-frame-server/internal/migration"
	"github.com/boloc/go-frame-server/pkg/frame"
	"github.com/boloc/go-frame-server/pkg/frame/config"
	"github.com/boloc/go-frame-server/pkg/logger"
)

// SetupMigration 在 AfterStart 里执行自动迁移（等 MySQL 已 Start）。
// 由 database.auto_migrate 控制，默认关闭。
func SetupMigration(f *frame.Frame, conf *config.ConfigComponent) {
	f.AfterStart(func(ctx context.Context) error {
		if !autoMigrateEnabled(conf) {
			logger.Info("migration: database.auto_migrate 未开启，跳过自动迁移")
			return nil
		}
		logger.Info("migration: 开始自动迁移")
		if err := migration.AutoMigrate(ctx); err != nil {
			return err
		}
		logger.Info("migration: 自动迁移完成")
		return nil
	})
}

// autoMigrateEnabled 读取 database.auto_migrate；未配置时默认关闭。
func autoMigrateEnabled(conf *config.ConfigComponent) bool {
	return conf.GetBool("database.auto_migrate")
}
