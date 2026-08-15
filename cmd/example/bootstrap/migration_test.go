package bootstrap

import (
	"testing"

	"github.com/boloc/go-frame-server/pkg/frame/config"
)

// TestAutoMigrateEnabledDefaultsToFalseWhenUnset 验证未配置 database.auto_migrate 时默认关闭。
func TestAutoMigrateEnabledDefaultsToFalseWhenUnset(t *testing.T) {
	conf := config.NewConfig("")
	if autoMigrateEnabled(conf) {
		t.Fatal("database.auto_migrate 未配置时应该默认关闭")
	}
}

// TestAutoMigrateEnabledRespectsExplicitTrue 验证 database.auto_migrate=true 时启用。
func TestAutoMigrateEnabledRespectsExplicitTrue(t *testing.T) {
	conf := config.NewConfig("")
	conf.GetViper().Set("database.auto_migrate", true)
	if !autoMigrateEnabled(conf) {
		t.Fatal("database.auto_migrate=true 时应该启用")
	}
}

// TestAutoMigrateEnabledRespectsExplicitFalse 验证 database.auto_migrate=false 时关闭。
func TestAutoMigrateEnabledRespectsExplicitFalse(t *testing.T) {
	conf := config.NewConfig("")
	conf.GetViper().Set("database.auto_migrate", false)
	if autoMigrateEnabled(conf) {
		t.Fatal("database.auto_migrate=false 时应该关闭")
	}
}
