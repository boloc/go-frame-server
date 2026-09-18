package route

import (
	examplemw "github.com/boloc/go-frame-server/internal/example/middleware"

	"github.com/boloc/go-frame-server/internal/example/handler"
	"github.com/boloc/go-frame-server/pkg/frame/idempotency"
	"github.com/gin-gonic/gin"
)

// registerOrderRoutes 演示 route -> handler -> validation -> logic -> repository 分层，
// 以及创建订单时用 idempotency 中间件（必带 Idempotency-Key，Redis 不可用时拒绝下单）。
// 本组挂了 RequireDemoToken：X-Demo-Token 既是演示鉴权，也是幂等 key 的 scope
// （同一用户重复提交重放，换用户同 key 不冲突）。
//
// 同一个 token + 同一个 Idempotency-Key，重复 POST 得到相同响应：
//
//	curl -X POST localhost:10006/api/orders \
//	  -H "X-Demo-Token: alice" \
//	  -H "Idempotency-Key: demo-key-1" \
//	  -H "Content-Type: application/json" \
//	  -d '{"product_id":1,"quantity":2}'
//
// 换 token、仍用 demo-key-1，不会命中 alice 的缓存，各自下一单：
//
//	curl -X POST localhost:10006/api/orders \
//	  -H "X-Demo-Token: bob" \
//	  -H "Idempotency-Key: demo-key-1" \
//	  -H "Content-Type: application/json" \
//	  -d '{"product_id":1,"quantity":2}'
func registerOrderRoutes(r *gin.Engine) {
	orders := r.Group("/api/orders")
	orders.Use(
		examplemw.RequireDemoToken(),
		idempotency.Middleware(
			idempotency.WithRequired(true),
			idempotency.WithFailOpen(false),
			idempotency.WithScopeFunc(func(c *gin.Context) string {
				return examplemw.DemoUser(c)
			}),
		),
	)
	{
		orders.POST("", handler.OrderCreate) // 创建订单
	}
}
