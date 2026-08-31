package bootstrap

import (
	"context"

	"github.com/boloc/go-frame-server/internal/migration"
	"github.com/boloc/go-frame-server/pkg/frame"
	"github.com/boloc/go-frame-server/pkg/frame/config"
	"github.com/boloc/go-frame-server/pkg/logger"
)

var _ frame.Component = (*migrationComponent)(nil)

// migrationComponent 在 MySQL Start 之后、缓存 Start 之前跑 AutoMigrate。
// 必须注册成组件而不是 AfterStart：缓存预热发生在组件 Start 里，AfterStart 太晚。
type migrationComponent struct {
	conf *config.ConfigComponent
}

func (c *migrationComponent) Start(ctx context.Context) error {
	if !autoMigrateEnabled(c.conf) {
		logger.Info("migration: database.auto_migrate 未开启，跳过自动迁移")
		return nil
	}
	logger.Info("migration: 开始自动迁移")
	if err := migration.AutoMigrate(ctx); err != nil {
		return err
	}
	logger.Info("migration: 自动迁移完成")
	return nil
}

func (c *migrationComponent) Stop(context.Context) error {
	return nil
}

// SetupMigration 注册自动迁移组件。调用位置必须在 SetupMySQL 之后、SetupCaches 之前。
// 由 database.auto_migrate 控制，默认关闭。
func SetupMigration(f *frame.Frame, conf *config.ConfigComponent) {
	f.RegisterComponent(&migrationComponent{conf: conf})
}

// autoMigrateEnabled 读取 database.auto_migrate；未配置时默认关闭。
func autoMigrateEnabled(conf *config.ConfigComponent) bool {
	return conf.GetBool("database.auto_migrate")
}
