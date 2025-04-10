package route

import (
	"github.com/gin-gonic/gin"
)

func RegisterRoutes(r *gin.Engine) {
	r.GET("/health", func(c *gin.Context) {
		// ctx := content.FromGin(c)
		// ctx.Set("test", "test")
		// // queryCtx := ctx.RequestQuery
		// // fmt.Println("打印ctx query", queryCtx)
		// // // // db := frame.GetDefaultDB()
		// // bodyCtx := ctx.RequestBody
		// // fmt.Println("打印ctx body", bodyCtx)
		// test, _ := ctx.Get("test")
		// Tss(c, test.(string))
		// if test, exists := ctx.Get("test"); exists {
		// 	fmt.Println("打印自定义参数test ", test)

		// } else {
		// 	fmt.Println("自定义参数test 不存在")
		// }
		// customData, exists := ctx.GetString("test")
		// Tss(c, customData)
		// fmt.Println("打印自定义数据string", customData, exists)
		// c.JSON(200, gin.H{"message": "ok"})

		c.JSON(200, gin.H{"message": "ok", "result": "ok"})

	})
}
