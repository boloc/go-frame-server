package bootstrap

import (
	"github.com/boloc/go-frame-server/cmd/example/cron"
	"github.com/boloc/go-frame-server/pkg/frame"
	"github.com/boloc/go-frame-server/pkg/frame/config"
	"github.com/boloc/go-frame-server/pkg/logger"
	"github.com/prometheus/client_golang/prometheus"
)

// SetupCron 把定时任务单例注册进 Frame，并注册执行指标。只应调用一次。
// 是否启动由 cron.enabled 控制（默认启用）。
func SetupCron(f *frame.Frame, conf *config.ConfigComponent) {
	if !cronEnabled(conf) {
		logger.Info("cron: disabled via config (cron.enabled=false), scheduler not started")
		return
	}
	f.RegisterSingleton("cron", cron.Component)
	prometheus.MustRegister(cron.Component.Collectors()...)
}

// cronEnabled 读取 cron.enabled；未配置时默认启用。
func cronEnabled(conf *config.ConfigComponent) bool {
	if !conf.GetViper().IsSet("cron.enabled") {
		return true
	}
	return conf.GetBool("cron.enabled")
}
