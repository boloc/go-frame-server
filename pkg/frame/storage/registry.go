package storage

import "sync/atomic"

// defaultClient 保存进程内默认的 S3Client 实例。跟 pkg/frame/components 里
// MySQL/Redis 那套"全局注册表 + Try 版访问器"是同一个模式：SetDefault 通常在
// bootstrap 阶段调用一次，业务代码用 TryDefault 取，不 panic，自己决定"没配置"
// 时怎么处理——对象存储通常是可选能力，不是每个应用都需要，不应该强制要求配置。
var defaultClient atomic.Pointer[S3Client]

// SetDefault 注册默认的 S3Client 实例。
func SetDefault(c *S3Client) {
	defaultClient.Store(c)
}

// TryDefault 返回默认实例；未注册（没调用过 SetDefault，通常意味着没配置对象存储）
// 时返回 (nil, false)，不 panic。
func TryDefault() (*S3Client, bool) {
	c := defaultClient.Load()
	if c == nil {
		return nil, false
	}
	return c, true
}
