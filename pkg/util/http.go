package util

import (
	"sync"
	"time"

	"github.com/go-resty/resty/v2"
)

// 全局共享的 HTTP 客户端，避免每次都创建新的
var (
	client     *resty.Client
	clientOnce sync.Once
)

// GetClient 获取单例的 HTTP 客户端
// 使用resty.New()创建新的客户端
func GetClient() *resty.Client {
	clientOnce.Do(func() {
		client = resty.New().
			SetTimeout(5 * time.Second) // 设置超时时间
	})
	return client
}
