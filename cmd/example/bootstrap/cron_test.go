package bootstrap

import (
	"testing"

	"github.com/boloc/go-frame-server/pkg/frame/config"
)

// TestCronEnabledDefaultsToTrueWhenUnset 验证未配置 cron.enabled 时默认启用。
func TestCronEnabledDefaultsToTrueWhenUnset(t *testing.T) {
	conf := config.NewConfig("")
	if !cronEnabled(conf) {
		t.Fatal("cron.enabled 未配置时应该默认启用")
	}
}

// TestCronEnabledRespectsExplicitFalse 验证 cron.enabled=false 时禁用。
func TestCronEnabledRespectsExplicitFalse(t *testing.T) {
	conf := config.NewConfig("")
	conf.GetViper().Set("cron.enabled", false)
	if cronEnabled(conf) {
		t.Fatal("cron.enabled=false 时应该禁用")
	}
}

// TestCronEnabledRespectsExplicitTrue 验证 cron.enabled=true 时启用。
func TestCronEnabledRespectsExplicitTrue(t *testing.T) {
	conf := config.NewConfig("")
	conf.GetViper().Set("cron.enabled", true)
	if !cronEnabled(conf) {
		t.Fatal("cron.enabled=true 时应该启用")
	}
}
