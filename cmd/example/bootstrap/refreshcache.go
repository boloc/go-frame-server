package bootstrap

import (
	"github.com/boloc/go-frame-server/v2/internal/example/cache"
	"github.com/boloc/go-frame-server/v2/pkg/frame"
	"github.com/prometheus/client_golang/prometheus"
)

// SetupCaches 把 internal/example/cache 里登记的所有双层刷新缓存注册进 Frame，并挂上
// refreshcache_refresh_total。新增缓存实例只需在那个包里加文件，这里不用改。
func SetupCaches(f *frame.Frame) {
	for _, c := range cache.All() {
		f.RegisterSingleton(c.Name, c.Cache)
		prometheus.MustRegister(c.Cache.Collectors()...)
	}
}
