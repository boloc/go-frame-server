package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// MaxBodyBytes 限制请求体大小。超过 n 字节时 Read 返回 error。
// 必须注册在 ContextMiddleware 等会读整包 body 的中间件之前。
func MaxBodyBytes(n int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, n)
		c.Next()
	}
}
