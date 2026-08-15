package bootstrap

import (
	"github.com/boloc/go-frame-server/pkg/frame/config"
	"github.com/boloc/go-frame-server/pkg/util"
)

// SetupTimezone 设置整个进程的默认时区（server.timezone，未配置时 UTC），影响
// time.Now()、日志时间戳等没有显式指定时区的代码。跟 MySQL 连接的时区无关——见
// util.BuildMysqlDSN，那边固定默认 UTC，不跟随这里的设置。
func SetupTimezone(conf *config.ConfigComponent) {
	if err := util.SetProcessTimezone(conf.GetString("server.timezone")); err != nil {
		panic("bootstrap: " + err.Error())
	}
}
