package content

import (
	"context"

	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
)

type RequestContext struct {
	GinContext   *gin.Context    // gin上下文
	RequestQuery *datatypes.JSON `json:"request_query"`
	RequestBody  *datatypes.JSON `json:"request_body"`
	// 添加一个通用的map用于存储自定义数据
	CustomData map[string]any `json:"custom_data"`
}

type contextKey string

// FromGin 直接从 gin.Context 获取自定义上下文
func FromGin(c *gin.Context) *RequestContext {
	rc := FromContext(c.Request.Context())
	rc.GinContext = c
	return rc
}

// Set 设置自定义数据
func (rc *RequestContext) Set(key string, value any) {
	if rc.CustomData == nil {
		rc.CustomData = make(map[string]any)
	}
	rc.CustomData[key] = value
}

// Get 获取自定义数据
func (rc *RequestContext) Get(key string) (any, bool) {
	if rc.CustomData == nil {
		return nil, false
	}
	value, exists := rc.CustomData[key]
	return value, exists
}

// GetString 获取字符串类型的值
func (rc *RequestContext) GetString(key string) (string, bool) {
	if value, exists := rc.Get(key); exists {
		if str, ok := value.(string); ok {
			return str, true
		}
	}
	return "", false
}

// GetInt 获取整数类型的值
func (rc *RequestContext) GetInt(key string) (int, bool) {
	if value, exists := rc.Get(key); exists {
		if num, ok := value.(int); ok {
			return num, true
		}
	}
	return 0, false
}

// FromContext 从标准 context 获取
func FromContext(ctx context.Context) *RequestContext {
	if v := ctx.Value(contextKey("request")); v != nil {
		return v.(*RequestContext)
	}
	return nil
}

// NewContext 创建新的上下文
func NewContext(ctx context.Context, rc *RequestContext) context.Context {
	return context.WithValue(ctx, contextKey("request"), rc)
}
