package middleware

import (
	"frame-server/pkg/frame/content"

	"github.com/gin-gonic/gin"
)

// ContextMiddleware 创建上下文中间件
func ContextMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		rc := &content.RequestContext{
			DeviceID: c.GetHeader("X-Device-ID"),
			UserID:   c.GetHeader("X-User-ID"),
			// 可以添加更多字段
		}
		c.Request = c.Request.WithContext(content.NewContext(c.Request.Context(), rc))
		c.Next()
	}
}
