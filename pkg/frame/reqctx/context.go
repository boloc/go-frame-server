// Package reqctx 把请求的 query/body 和通用元信息放进 context.Context。
package reqctx

import (
	"context"

	"github.com/gin-gonic/gin"
)

// RequestContext 单个请求的上下文。RequestID 不是幂等键。
type RequestContext struct {
	GinContext   *gin.Context
	RequestQuery []byte
	RequestBody  []byte
	RequestID    string
	ClientIP     string
	UserAgent    string
	CustomData   map[string]any
}

type contextKey string

const requestContextKey contextKey = "request"

// FromGin 从 gin.Context 取出 RequestContext，找不到时返回空对象，不会为 nil。
func FromGin(c *gin.Context) *RequestContext {
	rc := FromContext(c.Request.Context())
	rc.GinContext = c
	return rc
}

// Set 设置自定义数据。
func (rc *RequestContext) Set(key string, value any) {
	if rc.CustomData == nil {
		rc.CustomData = make(map[string]any)
	}
	rc.CustomData[key] = value
}

// Get 获取自定义数据。
func (rc *RequestContext) Get(key string) (any, bool) {
	if rc.CustomData == nil {
		return nil, false
	}
	value, exists := rc.CustomData[key]
	return value, exists
}

// GetString 获取字符串类型的值。
func (rc *RequestContext) GetString(key string) (string, bool) {
	if value, exists := rc.Get(key); exists {
		if str, ok := value.(string); ok {
			return str, true
		}
	}
	return "", false
}

// GetInt 获取整数类型的值。
func (rc *RequestContext) GetInt(key string) (int, bool) {
	if value, exists := rc.Get(key); exists {
		if num, ok := value.(int); ok {
			return num, true
		}
	}
	return 0, false
}

// FromContext 从 context 取出 RequestContext，找不到时返回空对象，不会为 nil。
func FromContext(ctx context.Context) *RequestContext {
	if v, ok := ctx.Value(requestContextKey).(*RequestContext); ok {
		return v
	}
	return &RequestContext{}
}

// NewContext 创建新的上下文。
func NewContext(ctx context.Context, rc *RequestContext) context.Context {
	return context.WithValue(ctx, requestContextKey, rc)
}
