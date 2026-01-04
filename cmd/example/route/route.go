package route

import (
	"github.com/boloc/go-frame-server/internal/example/handler"
	"github.com/gin-gonic/gin"
)

func RegisterRoutes(r *gin.Engine) {
	// 健康检查
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "ok"})
	})

	// 错误测试接口组
	errorTestHandler := handler.NewErrorTestHandler()
	testGroup := r.Group("/test/error")
	{
		testGroup.GET("/api", errorTestHandler.TestApiError)               // 测试 API 异常
		testGroup.GET("/sql", errorTestHandler.TestSqlError)               // 测试 SQL 异常
		testGroup.GET("/validation", errorTestHandler.TestValidationError) // 测试参数验证异常
		testGroup.GET("/client", errorTestHandler.TestClientError)         // 测试客户端请求异常
	}
}
