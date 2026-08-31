package bootstrap

import (
	"testing"

	"github.com/boloc/go-frame-server/pkg/frame/config"
)

func TestClickHouseEnabledDefaultsToTrueWhenUnset(t *testing.T) {
	conf := config.NewConfig("")
	if !clickhouseEnabled(conf) {
		t.Fatal("clickhouse.enabled 未配置时应该默认启用")
	}
}

func TestClickHouseEnabledRespectsExplicitFalse(t *testing.T) {
	conf := config.NewConfig("")
	conf.GetViper().Set("clickhouse.enabled", false)
	if clickhouseEnabled(conf) {
		t.Fatal("clickhouse.enabled=false 时应该禁用")
	}
}

func TestClickHouseEnabledRespectsExplicitTrue(t *testing.T) {
	conf := config.NewConfig("")
	conf.GetViper().Set("clickhouse.enabled", true)
	if !clickhouseEnabled(conf) {
		t.Fatal("clickhouse.enabled=true 时应该启用")
	}
}
