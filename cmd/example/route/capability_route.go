package route

import (
	examplemw "github.com/boloc/go-frame-server/internal/example/middleware"

	"github.com/boloc/go-frame-server/internal/example/handler"
	"github.com/gin-gonic/gin"
)

// registerCapabilityRoutes 演示框架响应/错误能力，挂在 /test 下，并给该组加演示 token 中间件。
//
//	curl localhost:10006/test/success                              # 401，缺 token
//	curl -H "X-Demo-Token: x" localhost:10006/test/success          # 200
func registerCapabilityRoutes(r *gin.Engine) {
	testGroup := r.Group("/test")
	testGroup.Use(examplemw.RequireDemoToken())
	{
		testGroup.GET("/success", handler.CapabilitySuccess)
		testGroup.GET("/not-found", handler.CapabilityNotFound)
		testGroup.GET("/not-found-with-status", handler.CapabilityNotFoundWithRealStatus)
		testGroup.GET("/unauthorized", handler.CapabilityUnauthorized)
		testGroup.GET("/database-error", handler.CapabilityDatabaseError)
		testGroup.GET("/business-error", handler.CapabilityBusinessError)
		testGroup.GET("/validation-error", handler.CapabilityValidationError)
		testGroup.GET("/panic", handler.CapabilityPanic)
		testGroup.GET("/error-source", handler.CapabilityErrorSource)
		testGroup.GET("/timezone", handler.CapabilityTimezone)
	}
}
