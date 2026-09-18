package constant

// refreshcache 的实例标识，不是库表里的 key。完整 Redis key 是
// {server.name}:cache:{这里}。
const (
	CacheKeyProductNotice      = "product:notice"
	CacheKeyProductActiveCount = "product:active_count"
)
