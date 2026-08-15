package middleware

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/boloc/go-frame-server/pkg/errs"
	"github.com/boloc/go-frame-server/pkg/frame/reqctx"
	"github.com/boloc/go-frame-server/pkg/frame/webx"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RequestIDHeader 请求/响应头中的请求 ID。这是单次调用的追踪 ID，不是幂等键。
const RequestIDHeader = "X-Request-Id"

// ContextMiddleware 把 query/body 写入 context，供 reqctx.FromContext/FromGin 取用。
// 会读完整 body；须排在 MaxBodyBytes 之后。读 body 失败返回 413。
func ContextMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		var requestQuery []byte
		if query := c.Request.URL.Query(); len(query) > 0 {
			requestQuery, _ = json.Marshal(query)
		}

		body, err := c.GetRawData()
		if err != nil {
			webx.FailWithStatus(c, errs.RequestTooLarge("请求体读取失败或超过大小限制"))
			c.Abort()
			return
		}
		// 读完后写回 Body，供后续 handler 继续读取。
		c.Request.Body = io.NopCloser(bytes.NewBuffer(body))

		// 复用上游已有的请求 ID，否则自行生成，并写入响应头。
		requestID := c.GetHeader(RequestIDHeader)
		if requestID == "" {
			requestID = uuid.NewString()
		}
		c.Writer.Header().Set(RequestIDHeader, requestID)

		rc := &reqctx.RequestContext{
			RequestQuery: requestQuery,
			RequestBody:  body,
			RequestID:    requestID,
			ClientIP:     c.ClientIP(), // 已由 gin TrustedProxies 解析，不要再解析 X-Forwarded-For
			UserAgent:    c.Request.UserAgent(),
		}
		c.Request = c.Request.WithContext(reqctx.NewContext(c.Request.Context(), rc))
		c.Next()
	}
}
