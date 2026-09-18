// Package cache 存放本示例的双层刷新只读缓存实例。
//
// 每个实例用 register 声明，声明即登记：bootstrap.SetupCaches 遍历 All() 统一注册进 Frame
// 并挂上刷新任务指标。新增一个缓存只需在本包加一个文件，不用再改 bootstrap。
package cache

import (
	"github.com/boloc/go-frame-server/pkg/frame"
	"github.com/boloc/go-frame-server/pkg/frame/refreshcache"
	"github.com/prometheus/client_golang/prometheus"
)

// Instance 是不同 T 的 refreshcache.Cache[T] 的公共视图：能随 Frame 启停，能交出刷新任务
// 指标。有了它，各实例才能放进同一个列表。
type Instance interface {
	frame.Component
	Collectors() []prometheus.Collector
}

// Registered 一个已登记的缓存。Name 同时用作 Frame 的单例键，进程内必须唯一。
type Registered struct {
	Name  string
	Cache Instance
}

var registry []Registered

// register 创建缓存实例并登记。只在包级变量初始化时调用，单 goroutine，不需要加锁。
func register[T any](name string, opts refreshcache.Options[T]) *refreshcache.Cache[T] {
	c := refreshcache.New(opts)
	registry = append(registry, Registered{Name: name, Cache: c})
	return c
}

// All 返回本包所有已登记的缓存，顺序是包内变量的初始化顺序。各缓存之间互不依赖，
// 所以启动顺序无关紧要。
func All() []Registered {
	return registry
}
