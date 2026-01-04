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

	// 测试接口组
	errorTestHandler := handler.NewErrorTestHandler()

	// 成功响应测试
	r.GET("/test/success", errorTestHandler.TestSuccess)
	r.GET("/test/custom", errorTestHandler.TestCustomResponse)

	// 错误响应测试
	testGroup := r.Group("/test/error")
	{
		testGroup.GET("/api", errorTestHandler.TestApiError)                   // 测试 API 异常
		testGroup.GET("/sql", errorTestHandler.TestSqlError)                   // 测试 SQL 异常
		testGroup.GET("/validation", errorTestHandler.TestValidationError)     // 测试参数验证异常
		testGroup.GET("/client", errorTestHandler.TestClientError)             // 测试客户端请求异常
		testGroup.GET("/notfound", errorTestHandler.TestNotFoundError)         // 测试未找到错误
		testGroup.GET("/unauthorized", errorTestHandler.TestUnauthorizedError) // 测试未授权错误
		testGroup.GET("/badrequest", errorTestHandler.TestBadRequestError)     // 测试错误请求
		testGroup.GET("/panic", errorTestHandler.TestPanicError)               // 测试 Panic 错误
	}

	// ==================== 产品接口示例 ====================
	productHandler := handler.NewProductHandler()
	r.GET("/api/products", productHandler.List) // 列表（分页）
}
