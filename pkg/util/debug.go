package util

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/boloc/go-frame-server/pkg/frame/reqctx"
)

// 本文件仅用于本地调试：打印到 stdout，可能含敏感字段，不要留在生产路径。

// PrintReqParams 调试用：打印当前请求的 method/path/query/body。
// 优先复用 reqctx 已读的 query/body；没有则自行读一次并写回 Body。
func PrintReqParams(c *gin.Context) {
	rc := reqctx.FromGin(c)

	query := rc.RequestQuery
	if len(query) == 0 {
		if q := c.Request.URL.Query(); len(q) > 0 {
			query, _ = json.Marshal(q)
		}
	}

	body := rc.RequestBody
	if len(body) == 0 && c.Request.Method != "GET" {
		if raw, err := c.GetRawData(); err == nil {
			body = raw
			c.Request.Body = io.NopCloser(bytes.NewBuffer(raw)) // 读完写回，不影响后续 handler
		}
	}

	fmt.Println(strings.Repeat("-", 60))
	fmt.Printf("请求: %s %s\n", c.Request.Method, c.Request.URL.Path)
	if len(query) > 0 {
		fmt.Printf("Query: %s\n", prettyJSON(query))
	}
	if len(body) > 0 {
		fmt.Printf("Body : %s\n", prettyJSON(body))
	}
	fmt.Println(strings.Repeat("-", 60))
}

// PrintFormat 调试用：打印对象的类型、Go 字面量视图和格式化 JSON。
func PrintFormat(obj any) {
	pretty, err := json.MarshalIndent(obj, "", "  ")

	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("类型  : %T\n", obj)
	fmt.Printf("Go视图: %#v\n", obj)
	if err != nil {
		fmt.Printf("JSON  : <序列化失败: %v>\n", err)
	} else {
		fmt.Printf("JSON  :\n%s\n", pretty)
	}
	fmt.Println(strings.Repeat("=", 60))
}

// prettyJSON 按缩进格式化 JSON；非法 JSON 原样返回。
func prettyJSON(raw []byte) string {
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		return string(raw)
	}
	return buf.String()
}
