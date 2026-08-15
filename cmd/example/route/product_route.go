package route

import (
	"time"

	"github.com/boloc/go-frame-server/internal/example/handler"
	"github.com/boloc/go-frame-server/pkg/frame/ratelimit"
	"github.com/gin-gonic/gin"
)

// registerProductRoutes 演示 route -> handler -> logic -> repository 分层，并挂限流中间件。
//
//	for i in $(seq 1 15); do curl -s -o /dev/null -w "%{http_code}\n" localhost:10006/api/products; done
func registerProductRoutes(r *gin.Engine) {
	products := r.Group("/api/products")
	products.Use(ratelimit.Middleware(ratelimit.WithLimit(10), ratelimit.WithWindow(time.Minute)))
	{
		products.GET("list", handler.ProductList) // 列表（分页）

		// 演示 refreshcache：读内存中的上架数量，不查库。
		products.GET("/summary", handler.ProductActiveCountSummary)

		// 演示 options：给表单提供状态下拉数据。
		products.GET("/options", handler.ProductOptions)

		// 演示跨两个命名 MySQL 实例读：详情在 default_db，公告在 config_db。
		products.GET("/:id", handler.ProductDetail)

		// 演示写主库；上面几个 GET 读从库。
		products.POST("/:id/status", handler.ProductUpdateStatus)
	}
}
