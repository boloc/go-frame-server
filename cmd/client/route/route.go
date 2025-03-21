package route

import (
	"context"
	"fmt"
	"frame-server/pkg/frame"
	"frame-server/pkg/frame/content"
	"frame-server/pkg/util"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(r *gin.Engine) {
	r.GET("/health", func(c *gin.Context) {

		panic("123")
		ctx := content.FromGin(c)
		fmt.Println("打印ctx", ctx)
		// // db := frame.GetDefaultDB()
		// dbSlave := frame.DefaultSlaveDB()
		// var v []map[string]any
		// // 查询表，不使用模型
		// dbSlave.Model(&model.ShortLinkRelationship{}).Find(&v)
		// // 转成json
		// json, _ := json.Marshal(v)
		// fmt.Println("打印db结果", string(json))

		// 获取redis
		redis := frame.GetRedis()
		redis.Set(context.Background(), "test", "周泽", util.RandomTTL(999))
		a, _ := redis.Get(context.Background(), "test").Result()
		fmt.Println("打印redis结果", a)
		c.JSON(200, gin.H{"message": a})
	})
}
