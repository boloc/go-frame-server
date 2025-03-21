package middleware

import (
	"frame-server/pkg/response"

	"github.com/gin-gonic/gin"
)

// panic处理中间件
func RecoveryMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				// 返回自定义错误响应
				response.ErrorPanic(c, any(err))

				// 终止后续中间件
				c.Abort()
			}
		}()

		// 处理请求
		c.Next()
	}
}
