// Package migration 是本示例的数据库迁移入口：GORM AutoMigrate，由 database.auto_migrate 控制。
package migration

import (
	"context"
	"fmt"

	"github.com/boloc/go-frame-server/internal/example/constant"
	"github.com/boloc/go-frame-server/internal/model"
	"github.com/boloc/go-frame-server/pkg/frame/components"
)

// perInstanceModels 描述每个命名 MySQL 实例要迁移的模型。
var perInstanceModels = map[string][]any{
	constant.MySQLDefaultDB: {&model.Product{}},
	constant.MySQLConfigDB:  {&model.SystemConfig{}},
	constant.MySQLLogDB:     {&model.OperationLog{}},
}

// AutoMigrate 对每个命名 MySQL 实例分别执行 AutoMigrate，应在对应组件 Start 成功后调用。
func AutoMigrate(ctx context.Context) error {
	for name, models := range perInstanceModels {
		db, ok := components.TryMasterDB(name)
		if !ok {
			return fmt.Errorf("migration: MySQL 实例 %q 尚未连接，无法执行迁移", name)
		}
		if err := db.WithContext(ctx).AutoMigrate(models...); err != nil {
			return fmt.Errorf("migration: 实例 %q 迁移失败: %w", name, err)
		}
	}
	return nil
}
