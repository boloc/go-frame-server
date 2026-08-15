package handler

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"time"

	"github.com/boloc/go-frame-server/pkg/errs"
	"github.com/boloc/go-frame-server/pkg/frame/webx"
	"github.com/boloc/go-frame-server/pkg/util"
	"github.com/gin-gonic/gin"

	"github.com/go-resty/resty/v2"
)

// 这一组函数演示出站 HTTP 客户端的重试与命名超时，路由挂在 /test/http-client 下。
// 每个接口用 httptest 起一个临时下游，不依赖真实外部网络。

// flakyBackend 起一个前 failTimes 次返回 500、之后返回 200 的测试后端，返回 server
// 和一个能随时读取"总共被访问了多少次"的函数。调用方负责在用完之后 Close server。
func flakyBackend(failTimes int32) (srv *httptest.Server, hits func() int32) {
	var attempts int32
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&attempts, 1) <= failTimes {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	return srv, func() int32 { return atomic.LoadInt32(&attempts) }
}

// HTTPClientRetryOnIdempotent 演示默认重试策略对幂等方法生效：下游前 2 次返回 500，
// GET 是幂等方法，客户端会自动重试，第 3 次成功。backend_hits 应该是 3。
// GET /test/http-client/retry-get
func HTTPClientRetryOnIdempotent(c *gin.Context) {
	srv, hits := flakyBackend(2)
	defer srv.Close()

	// 用一个重试等待时间很短的命名客户端，避免演示接口被默认的 200ms/2s 退避拖慢；
	// RetryCount 不设，沿用默认值 2，正好对应 flakyBackend(2) 的失败次数。
	client := util.GetNamedClient("demo-retry-get", util.HTTPClientOptions{
		RetryWaitTime: 10 * time.Millisecond, RetryMaxWaitTime: 50 * time.Millisecond,
	})
	resp, err := client.R().Get(srv.URL)
	if err != nil {
		webx.Fail(c, errs.Dependency(err))
		return
	}
	webx.Success(c, gin.H{
		"status_code":  resp.StatusCode(),
		"backend_hits": hits(),
		"hint":         "GET 是幂等方法，前 2 次 500 会被自动重试，backend_hits 应该是 3",
	})
}

// HTTPClientNoRetryOnPost 演示 POST 默认不重试：backend_hits 应为 1，状态码是下游的 500。
// GET /test/http-client/no-retry-post
func HTTPClientNoRetryOnPost(c *gin.Context) {
	srv, hits := flakyBackend(2)
	defer srv.Close()

	resp, err := util.GetClient().R().Post(srv.URL)
	webx.Success(c, gin.H{
		"status_code":  statusOrZero(resp),
		"error":        errString(err),
		"backend_hits": hits(),
		"hint":         "POST 默认不重试（避免重复提交），backend_hits 应该是 1，且状态码是下游的 500",
	})
}

// HTTPClientRetryAllowNonIdempotent 演示显式放开非幂等重试后，POST 也会重试到成功。
// GET /test/http-client/retry-post-allowed
func HTTPClientRetryAllowNonIdempotent(c *gin.Context) {
	srv, hits := flakyBackend(2)
	defer srv.Close()

	client := util.GetNamedClient("demo-retry-post-allowed", util.HTTPClientOptions{
		RetryAllowNonIdempotent: true,
		RetryWaitTime:           10 * time.Millisecond,
		RetryMaxWaitTime:        50 * time.Millisecond,
	})
	resp, err := client.R().Post(srv.URL)
	if err != nil {
		webx.Fail(c, errs.Dependency(err))
		return
	}
	webx.Success(c, gin.H{
		"status_code":  resp.StatusCode(),
		"backend_hits": hits(),
		"hint":         "显式 RetryAllowNonIdempotent=true 之后 POST 也会重试，backend_hits 应该是 3",
	})
}

// HTTPClientNamedTimeout 演示按 name 隔离超时：短超时客户端先失败，默认客户端能等到 200。
// GET /test/http-client/named-timeout
func HTTPClientNamedTimeout(c *gin.Context) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(150 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	fast := util.GetNamedClient("demo-fast-timeout", util.HTTPClientOptions{
		Timeout: 30 * time.Millisecond, RetryCount: util.NoRetry,
	})
	_, fastErr := fast.R().Get(srv.URL)
	resp, defaultErr := util.GetClient().R().Get(srv.URL)

	webx.Success(c, gin.H{
		"fast_client_timeout_error": errString(fastErr),
		"default_client_status":     statusOrZero(resp),
		"default_client_error":      errString(defaultErr),
		"hint":                      "30ms 超时的命名客户端应该先超时失败，默认客户端（5s 超时）应该等到下游 150ms 之后正常返回 200",
	})
}

func statusOrZero(resp *resty.Response) int {
	if resp == nil {
		return 0
	}
	return resp.StatusCode()
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
