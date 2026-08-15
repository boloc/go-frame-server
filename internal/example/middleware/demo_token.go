// Package middleware 存放本示例自己的业务中间件。
package middleware

import (
	"github.com/boloc/go-frame-server/pkg/errs"
	"github.com/boloc/go-frame-server/pkg/frame/webx"
	"github.com/gin-gonic/gin"
)

// RequireDemoToken 演示给一组路由单独加中间件：请求头需带 X-Demo-Token，不是真实鉴权。
func RequireDemoToken() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetHeader("X-Demo-Token") == "" {
			webx.Fail(c, errs.Unauthorized("演示用：请带上 X-Demo-Token 请求头才能访问 /test 下的接口"))
			c.Abort() // 中断链路，不再执行后面的 handler
			return
		}
		c.Next()
	}
}
