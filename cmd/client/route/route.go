package route

import (
	"fmt"

	"github.com/boloc/go-frame-server/pkg/frame/content"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(r *gin.Engine) {
	r.GET("/health", func(c *gin.Context) {
		ctx := content.FromGin(c)
		queryCtx := ctx.RequestQuery
		fmt.Println("打印ctx query", queryCtx)
		// // db := frame.GetDefaultDB()
		bodyCtx := ctx.RequestBody
		fmt.Println("打印ctx body", bodyCtx)
		c.JSON(200, gin.H{"message": "ok"})
	})
}
