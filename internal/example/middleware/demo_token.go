// Package middleware 存放本示例自己的业务中间件。
package middleware

import (
	"github.com/boloc/go-frame-server/pkg/errs"
	"github.com/boloc/go-frame-server/pkg/frame/webx"
	"github.com/gin-gonic/gin"
)

const demoUserKey = "example.demo_user"

// RequireDemoToken 演示给一组路由单独加中间件：请求头需带 X-Demo-Token，不是真实鉴权。
// 通过后把 token 本身当作演示用户标识写入 gin.Context，供幂等 scope 等读取。
func RequireDemoToken() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.GetHeader("X-Demo-Token")
		if token == "" {
			webx.Fail(c, errs.Unauthorized("演示用：请带上 X-Demo-Token 请求头"))
			c.Abort() // 中断链路，不再执行后面的 handler
			return
		}
		c.Set(demoUserKey, token)
		c.Next()
	}
}

// DemoUser 返回 RequireDemoToken 写入的演示用户标识（即 X-Demo-Token 的值）。
// 中间件未挂载或尚未执行时返回空字符串。
func DemoUser(c *gin.Context) string {
	v, ok := c.Get(demoUserKey)
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}
