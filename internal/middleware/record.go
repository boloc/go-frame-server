package middleware

import (
	"go-frame-server/pkg/requests"

	"github.com/gin-gonic/gin"
)

// 自定义ResponseWriter来捕获响应大小
type responseWriter struct {
	gin.ResponseWriter
	bodySize int
}

func (w *responseWriter) Write(b []byte) (int, error) {
	size, err := w.ResponseWriter.Write(b)
	w.bodySize += size
	return size, err
}

func (w *responseWriter) WriteString(s string) (int, error) {
	size, err := w.ResponseWriter.WriteString(s)
	w.bodySize += size
	return size, err
}

// RecordMiddleware 记录请求中间件
func RecordMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// start := time.Now()

		// // 获取请求包大小
		// requestSize := 0
		// if c.Request.ContentLength > 0 {
		// 	requestSize = int(c.Request.ContentLength)
		// }

		// 创建自定义ResponseWriter来跟踪响应大小
		customWriter := &responseWriter{
			ResponseWriter: c.Writer,
			bodySize:       0,
		}
		c.Writer = customWriter

		// 设置请求上下文
		reqs := requests.NewRequests(c)
		c.Set("requests", reqs)

		// 处理请求
		c.Next()

		// 请求处理完毕，记录各项指标
		// duration := time.Since(start)

		// 记录响应时间
		// monitor.ObserveLatency(c.Request.URL.Path, duration)

		// // 可以添加日志输出，帮助调试
		// fmt.Println("打印duration", duration)
		// fmt.Printf("打印：[%s] %s %d %dms (req: %d bytes, resp: %d bytes)\n",
		// 	c.Request.Method, c.Request.URL.Path, c.Writer.Status(), duration.Milliseconds(),
		// 	requestSize, customWriter.bodySize)
	}
}
