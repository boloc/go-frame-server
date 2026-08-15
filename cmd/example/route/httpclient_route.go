package route

import (
	examplemw "github.com/boloc/go-frame-server/internal/example/middleware"

	"github.com/boloc/go-frame-server/internal/example/handler"
	"github.com/gin-gonic/gin"
)

// registerHTTPClientRoutes 演示出站 HTTP 客户端的重试与命名超时，挂在 /test/http-client 下。
//
//	curl -H "X-Demo-Token: x" localhost:10006/test/http-client/retry-get
//	curl -H "X-Demo-Token: x" localhost:10006/test/http-client/no-retry-post
//	curl -H "X-Demo-Token: x" localhost:10006/test/http-client/retry-post-allowed
//	curl -H "X-Demo-Token: x" localhost:10006/test/http-client/named-timeout
func registerHTTPClientRoutes(r *gin.Engine) {
	group := r.Group("/test/http-client")
	group.Use(examplemw.RequireDemoToken())
	{
		group.GET("/retry-get", handler.HTTPClientRetryOnIdempotent)
		group.GET("/no-retry-post", handler.HTTPClientNoRetryOnPost)
		group.GET("/retry-post-allowed", handler.HTTPClientRetryAllowNonIdempotent)
		group.GET("/named-timeout", handler.HTTPClientNamedTimeout)
	}
}
