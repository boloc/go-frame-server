package content

import (
	"context"

	"github.com/gin-gonic/gin"
)

type RequestContext struct {
	DeviceID string
	UserID   string
	// 可以根据需要添加其他字段
}

type contextKey string

// FromGin 直接从 gin.Context 获取自定义上下文
func FromGin(c *gin.Context) *RequestContext {
	return FromContext(c.Request.Context())
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
