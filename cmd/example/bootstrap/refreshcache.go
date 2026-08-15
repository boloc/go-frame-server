package bootstrap

import (
	examplecache "github.com/boloc/go-frame-server/internal/example/cache"
	"github.com/boloc/go-frame-server/pkg/frame"
	"github.com/prometheus/client_golang/prometheus"
)

// SetupCaches 是这个应用所有 refreshcache.Cache 实例的唯一入口，bootstrap.Setup 只调
// 这一个函数。新增一个缓存实例，在这里加一个 setupXxxCache 函数、补一行调用即可。
func SetupCaches(f *frame.Frame) {
	setupProductActiveCountCache(f)
	setupProductNoticeCache(f)
}

// setupProductActiveCountCache 把产品上架数量的双层刷新缓存注册进 Frame，并挂上执行指标。
func setupProductActiveCountCache(f *frame.Frame) {
	f.RegisterSingleton("product-active-count-cache", examplecache.ProductActiveCount)
	prometheus.MustRegister(examplecache.ProductActiveCount.Collectors()...)
}

// setupProductNoticeCache 把产品详情页公告的双层刷新缓存注册进 Frame，并挂上执行指标。
func setupProductNoticeCache(f *frame.Frame) {
	f.RegisterSingleton("product-notice-cache", examplecache.ProductNotice)
	prometheus.MustRegister(examplecache.ProductNotice.Collectors()...)
}
