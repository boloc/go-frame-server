package util

import (
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"

	"github.com/boloc/go-frame-server/pkg/logger"
)

// HTTPClientOptions 覆盖默认的超时/重试/连接池参数。零值回落到 defaultHTTPClientOptions。
type HTTPClientOptions struct {
	Timeout          time.Duration // 单次请求超时，默认 5s
	RetryWaitTime    time.Duration // 重试的初始等待时间，默认 200ms（resty 内置指数退避）
	RetryMaxWaitTime time.Duration // 重试等待时间上限，默认 2s

	// RetryCount 失败重试次数，默认 2。
	// 不能传 0 表示不重试（0 是零值，会回落到默认 2）；显式禁用请传 NoRetry（-1）。
	// body 为 io.Reader 时 resty 默认不缓冲，重试会发出空 body；此类请求应对该次请求 SetRetryCount(0)。
	RetryCount int

	// RetryAllowNonIdempotent 是否允许对 POST/PATCH 等非幂等方法重试，默认 false。
	RetryAllowNonIdempotent bool

	MaxIdleConns        int           // 进程级最大空闲连接数，默认 100
	MaxIdleConnsPerHost int           // 每个 host 的最大空闲连接数，默认 20
	IdleConnTimeout     time.Duration // 空闲连接在池里最长保留时间，默认 90s

	Debug bool // 是否打开 resty 的请求/响应详情日志（走 pkg/logger.Debug）
}

// NoRetry 传给 HTTPClientOptions.RetryCount，显式表示不重试。不能传字面量 0。
const NoRetry = -1

func defaultHTTPClientOptions() HTTPClientOptions {
	return HTTPClientOptions{
		Timeout:             5 * time.Second,
		RetryCount:          2,
		RetryWaitTime:       200 * time.Millisecond,
		RetryMaxWaitTime:    2 * time.Second,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 20,
		IdleConnTimeout:     90 * time.Second,
	}
}

// applyDefaults 把 opts 里没设置（零值）的字段回落到默认值，不修改 opts 本身。
func (o HTTPClientOptions) applyDefaults() HTTPClientOptions {
	def := defaultHTTPClientOptions()
	if o.Timeout <= 0 {
		o.Timeout = def.Timeout
	}
	switch {
	case o.RetryCount == 0: // 零值 = 未设置，回落到默认值
		o.RetryCount = def.RetryCount
	case o.RetryCount < 0: // NoRetry（或任何负数）表示不重试
		o.RetryCount = 0
	}
	if o.RetryWaitTime <= 0 {
		o.RetryWaitTime = def.RetryWaitTime
	}
	if o.RetryMaxWaitTime <= 0 {
		o.RetryMaxWaitTime = def.RetryMaxWaitTime
	}
	if o.MaxIdleConns <= 0 {
		o.MaxIdleConns = def.MaxIdleConns
	}
	if o.MaxIdleConnsPerHost <= 0 {
		o.MaxIdleConnsPerHost = def.MaxIdleConnsPerHost
	}
	if o.IdleConnTimeout <= 0 {
		o.IdleConnTimeout = def.IdleConnTimeout
	}
	return o
}

// idempotentHTTPMethods 是 RFC 9110 9.2.2 定义的幂等方法集合。
var idempotentHTTPMethods = map[string]bool{
	http.MethodGet:     true,
	http.MethodHead:    true,
	http.MethodPut:     true,
	http.MethodDelete:  true,
	http.MethodOptions: true,
	http.MethodTrace:   true,
}

// restyZapLogger 把 resty 内部日志转到 pkg/logger。
type restyZapLogger struct{}

func (restyZapLogger) Errorf(format string, v ...any) { logger.Error(fmt.Sprintf(format, v...)) }
func (restyZapLogger) Warnf(format string, v ...any)  { logger.Warn(fmt.Sprintf(format, v...)) }
func (restyZapLogger) Debugf(format string, v ...any) { logger.Debug(fmt.Sprintf(format, v...)) }

// newRestyClient 按 opts 构建 resty client。应复用返回值，不要每次请求 New。
func newRestyClient(opts HTTPClientOptions) *resty.Client {
	opts = opts.applyDefaults()

	transport := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		MaxIdleConns:        opts.MaxIdleConns,
		MaxIdleConnsPerHost: opts.MaxIdleConnsPerHost,
		IdleConnTimeout:     opts.IdleConnTimeout,
		DialContext:         (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
	}

	c := resty.New().
		SetTransport(transport).
		SetTimeout(opts.Timeout).
		SetRetryCount(opts.RetryCount).
		SetRetryWaitTime(opts.RetryWaitTime).
		SetRetryMaxWaitTime(opts.RetryMaxWaitTime).
		AddRetryCondition(func(r *resty.Response, err error) bool {
			if r != nil && r.Request != nil && !opts.RetryAllowNonIdempotent && !idempotentHTTPMethods[r.Request.Method] {
				return false // 非幂等方法且未显式放开，不重试
			}
			// 网络错误或 5xx 才重试；4xx 不重试。
			return err != nil || (r != nil && r.StatusCode() >= http.StatusInternalServerError)
		}).
		SetLogger(restyZapLogger{})

	if opts.Debug {
		c.SetDebug(true)
	}

	return c
}

var (
	clientsMu sync.RWMutex
	clients   = make(map[string]*resty.Client)
)

// defaultClientName 是 GetClient() 在内部 clients map 里使用的 key，业务代码不需要关心它。
const defaultClientName = "__default__"

// GetClient 返回默认单例 HTTP 客户端（5s 超时，网络错误/5xx 重试 2 次，非幂等方法与 4xx 不重试）。
// 需要不同策略时用 GetNamedClient。
func GetClient() *resty.Client {
	return GetNamedClient(defaultClientName, HTTPClientOptions{})
}

// GetNamedClient 按 name 获取独立配置的 HTTP 客户端。同一 name 只创建一次，opts 仅首次生效。
func GetNamedClient(name string, opts HTTPClientOptions) *resty.Client {
	clientsMu.RLock()
	c, ok := clients[name]
	clientsMu.RUnlock()
	if ok {
		return c
	}

	clientsMu.Lock()
	defer clientsMu.Unlock()
	if c, ok = clients[name]; ok { // 双检查：写锁前可能已被其它 goroutine 创建
		return c
	}
	c = newRestyClient(opts)
	clients[name] = c
	return c
}
