package route

import (
	"github.com/boloc/go-frame-server/internal/example/handler"
	"github.com/boloc/go-frame-server/pkg/frame/idempotency"
	"github.com/gin-gonic/gin"
)

// registerOrderRoutes 演示 route -> handler -> validation -> logic -> repository 分层，
// 以及创建订单时用 idempotency 中间件（必带 Idempotency-Key，Redis 不可用时拒绝下单）。
//
//	curl -X POST localhost:10006/api/orders \
//	  -H "Idempotency-Key: demo-key-1" \
//	  -H "Content-Type: application/json" \
//	  -d '{"product_id":1,"quantity":2}'
func registerOrderRoutes(r *gin.Engine) {
	orders := r.Group("/api/orders")
	orders.Use(
		idempotency.Middleware(idempotency.WithRequired(true), idempotency.WithFailOpen(false)),
	)
	{
		orders.POST("", handler.OrderCreate) // 创建订单
	}
}
