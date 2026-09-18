// Package rediskey 统一本进程写进 Redis 的所有 key 的命名空间，是查"某个 key 由哪几段
// 拼出来"的唯一出处。
//
// 完整 key 按冒号分段，从外到内依次是 命名空间 -> 能力 -> 实例标识：
//
//	幂等      Namespace() + SegIdempotency + 路由 FullPath + [":" + scope] + ":" + 客户端幂等键
//	限流      Namespace() + SegRateLimit   + 路由 FullPath + ":" + ClientIP
//	任务锁    Namespace() + SegCronLock    + 调度器名（cron.NewComponent 的 name）+ ":" + Task.Name
//	刷新缓存  Namespace() + SegCache       + refreshcache.Options.Key
//
// 命名空间由应用在装配阶段用 SetNamespace 注入一次（示例取 server.name，见
// cmd/example/bootstrap/redis.go）。框架故意不提供默认值：admin / api 这类多个应用共用
// 同一个 Redis db 时，一个写死的默认值会让它们的键空间悄悄重叠，而这种问题只在线上暴露。
//
// 这些 key 都是懒拼的——幂等/限流在中间件构造时取前缀（路由注册发生在 GinComponent.Start
// 里），任务锁和缓存 key 在真正读写 Redis 时才拼——所以即便持有它们的是包级变量（构造早于
// 配置加载），在 bootstrap 里设置一次也能全局生效。
package rediskey

import (
	"strings"
	"sync/atomic"
)

// 四类能力各自的 key 段，拼在命名空间之后。
const (
	SegIdempotency = "idemp:"     // 幂等
	SegRateLimit   = "ratelimit:" // 限流
	SegCronLock    = "cron:lock:" // 任务锁
	SegCache       = "cache:"     // 刷新缓存
)

// namespace 一旦设置就不再变化，所以"读到值"必然意味着它已经被设置过，
// 不需要额外的冻结标记来防止 key 拼到一半被换掉。
var namespace atomic.Pointer[string]

// SetNamespace 设置命名空间，每个进程只能设置一次，且必须在注册组件之前。
// ns 不带尾部冒号时自动补上。
//
// ns 为空或重复设置都直接 panic：这是启动期就该发现的装配错误，不能让它退化成
// "两个应用共用一套 key" 这种只有线上才会暴露的问题。
func SetNamespace(ns string) {
	ns = strings.TrimSpace(ns)
	if ns == "" {
		panic("rediskey: 命名空间不能为空，应用需要传一个稳定的应用标识（示例用 server.name）")
	}
	ns = strings.TrimSuffix(ns, ":") + ":"
	if !namespace.CompareAndSwap(nil, &ns) {
		panic("rediskey: 命名空间已经是 " + *namespace.Load() + "，每个进程只能设置一次")
	}
}

// IsSet 报告命名空间是否已经设置，供组件在 Start 里把"应用忘了设置"变成启动错误
// 而不是运行期 panic。
func IsSet() bool {
	return namespace.Load() != nil
}

// Namespace 返回已设置的命名空间，带尾部冒号。未设置时 panic。
func Namespace() string {
	ns := namespace.Load()
	if ns == nil {
		panic("rediskey: 命名空间尚未设置，应用需要在注册组件之前调用 rediskey.SetNamespace")
	}
	return *ns
}

// Prefix 返回某一类能力的 key 前缀：Namespace() + segment。
func Prefix(segment string) string {
	return Namespace() + segment
}

// App 拼一个业务 key：Namespace() + 各段用冒号连接。业务代码自己 Set/Get 的 key 框架不会
// 自动加命名空间，用它就能和其它应用隔开——换一个 server.name 就是另一份数据。
//
//	rediskey.App("product", "123", "stock") // => "frame-example:product:123:stock"
//
// 段为空直接 panic，这是编程错误，不能让它拼出 "ns::stock" 这种 key。
func App(segments ...string) string {
	if len(segments) == 0 {
		panic("rediskey: App 至少需要一个段")
	}
	for _, seg := range segments {
		if strings.TrimSpace(seg) == "" {
			panic("rediskey: App 的段不能为空")
		}
	}
	return Namespace() + strings.Join(segments, ":")
}
