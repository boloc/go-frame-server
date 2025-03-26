package middleware

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/boloc/go-frame-server/pkg/frame/content"
	"gorm.io/datatypes"

	"github.com/gin-gonic/gin"
)

// ContextMiddleware 创建上下文中间件
func ContextMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 判断方法类型
		var (
			requestQuery datatypes.JSON
			requestBody  datatypes.JSON
		)

		// 打印GET请求参数
		query := c.Request.URL.Query()
		requestQuery, _ = json.Marshal(query)
		if string(requestQuery) == "{}" { // 如果请求参数为空，则设置为nil
			requestQuery = nil
		}

		// 打印POST请求参数
		body, _ := c.GetRawData()
		requestBody = datatypes.JSON(body)
		if len(body) == 0 || string(requestBody) == "{}" { // 如果请求参数为空，则设置为nil
			requestBody = nil
		}

		// 重置请求体 PS:为了不阻碍后续处理中还需要用到原始请求体，将数据重新设置回去
		c.Request.Body = io.NopCloser(bytes.NewBuffer(body))

		rc := &content.RequestContext{
			RequestQuery: &requestQuery,
			RequestBody:  &requestBody,
			CustomData:   make(map[string]any), // 初始化CustomData
		}
		c.Request = c.Request.WithContext(content.NewContext(c.Request.Context(), rc))
		c.Next()
	}
}
