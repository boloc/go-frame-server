package middleware

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/boloc/go-frame-server/v2/pkg/logger"
	"github.com/gin-gonic/gin"
)

// defaultParamMaxBytes 是 query、body 各自最多记录的字节数。超出部分截断，
// 防止有人往参数里塞超长字符串把日志撑大。4KB 能完整记下常见的后台表单（几段文字加十几个图片 URL）。
const defaultParamMaxBytes = 4096

const maskedValue = "***"

// paramMasker 按业务传入的字段名脱敏 query、表单和 JSON。没有字段名时原样输出。
type paramMasker struct {
	keys []string
	// JSON 解析不了（被截断或本身不合法）时按文本替换："<key>": "x" -> "<key>": "***"。
	jsonRe *regexp.Regexp
	// query 和表单：<key>=x -> <key>=***。
	formRe *regexp.Regexp
}

func newParamMasker(keys []string) *paramMasker {
	quoted := make([]string, 0, len(keys))
	lower := make([]string, 0, len(keys))
	for _, k := range keys {
		if k == "" {
			continue
		}
		lower = append(lower, strings.ToLower(k))
		quoted = append(quoted, regexp.QuoteMeta(k))
	}
	if len(quoted) == 0 {
		return &paramMasker{}
	}
	alt := strings.Join(quoted, "|")
	return &paramMasker{
		keys: lower,
		// 值是对象/数组（{ 或 [ 开头）时不匹配，里面的字段由各自的键再匹配。
		jsonRe: regexp.MustCompile(`(?i)("[^"]*(?:` + alt + `)[^"]*"\s*:\s*)("(?:[^"\\]|\\.)*"?|[^,}\]\s\[{"]+)`),
		formRe: regexp.MustCompile(`(?i)((?:^|&)[^=&]*(?:` + alt + `)[^=&]*=)[^&]*`),
	}
}

func (m *paramMasker) sensitive(key string) bool {
	k := strings.ToLower(key)
	for _, s := range m.keys {
		if strings.Contains(k, s) {
			return true
		}
	}
	return false
}

func (m *paramMasker) form(s string) string {
	if m.formRe == nil {
		return s
	}
	return m.formRe.ReplaceAllString(s, "${1}"+maskedValue)
}

// 完整合法的 JSON 解析后逐个字段脱敏再序列化（键会按字母序输出）；解析不了时按文本替换。
func (m *paramMasker) json(b []byte) string {
	if m.jsonRe == nil {
		return string(b)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil || dec.More() {
		return m.jsonRe.ReplaceAllString(string(b), `${1}"`+maskedValue+`"`)
	}
	m.maskValue(v)
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return m.jsonRe.ReplaceAllString(string(b), `${1}"`+maskedValue+`"`)
	}
	return strings.TrimSuffix(out.String(), "\n")
}

func (m *paramMasker) maskValue(v any) {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			switch val.(type) {
			case map[string]any, []any:
				// 只脱敏标量值；值是对象或数组时继续往里找，保留结构。
				m.maskValue(val)
			default:
				if m.sensitive(k) {
					t[k] = maskedValue
				}
			}
		}
	case []any:
		for _, val := range t {
			m.maskValue(val)
		}
	}
}

// bodyCapture 在 handler 读 body 时顺带留一份前 limit 字节，不额外读、不改变 handler 读到的内容。
type bodyCapture struct {
	io.ReadCloser
	buf   []byte
	limit int
	total int
}

func (b *bodyCapture) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 {
		b.total += n
		if room := b.limit - len(b.buf); room > 0 {
			b.buf = append(b.buf, p[:min(n, room)]...)
		}
	}
	return n, err
}

type bodyKind int

const (
	bodyNone bodyKind = iota
	bodyJSON
	bodyForm
)

// 只记 JSON 和表单；文件上传等二进制 body 不记。
func loggableBodyKind(c *gin.Context) bodyKind {
	if c.Request.Body == nil || c.Request.Body == http.NoBody {
		return bodyNone
	}
	ct := c.ContentType()
	switch {
	case ct == "application/json" || strings.HasSuffix(ct, "+json"):
		return bodyJSON
	case ct == "application/x-www-form-urlencoded":
		return bodyForm
	default:
		return bodyNone
	}
}

// 请求开始时调用：可记录的 body 换成 bodyCapture。返回 nil 表示这个请求不记 body。
func captureBody(c *gin.Context, limit int) (*bodyCapture, bodyKind) {
	if limit <= 0 {
		return nil, bodyNone
	}
	kind := loggableBodyKind(c)
	if kind == bodyNone {
		return nil, bodyNone
	}
	bc := &bodyCapture{ReadCloser: c.Request.Body, limit: limit}
	c.Request.Body = bc
	return bc, kind
}

// 请求结束时生成 query、body 字段。都已截断到 limit 并脱敏。
func paramFields(c *gin.Context, limit int, m *paramMasker, bc *bodyCapture, kind bodyKind) []logger.Field {
	if limit <= 0 {
		return nil
	}
	var fields []logger.Field
	if raw := c.Request.URL.RawQuery; raw != "" {
		if len(raw) <= limit {
			fields = append(fields, logger.String("query", m.form(raw)))
		} else {
			q := m.form(string(runePrefix([]byte(raw[:limit])))) + truncatedMark(len(raw))
			fields = append(fields, logger.String("query", q), logger.Bool("query_truncated", true))
		}
	}
	if bc != nil && len(bc.buf) > 0 {
		// handler 不一定把 body 读到 EOF，总大小优先用 Content-Length。
		total := max(bc.total, int(c.Request.ContentLength))
		truncated := total > len(bc.buf)
		buf := bc.buf
		if truncated {
			buf = runePrefix(buf)
		}
		var body string
		if kind == bodyJSON {
			body = m.json(buf)
		} else {
			body = m.form(string(buf))
		}
		if truncated {
			fields = append(fields, logger.String("body", body+truncatedMark(total)), logger.Bool("body_truncated", true))
		} else {
			fields = append(fields, logger.String("body", body))
		}
	}
	return fields
}

// 截断后追加在值末尾，只看这个字段也知道被截过、原来多大。
func truncatedMark(total int) string {
	return fmt.Sprintf("…[truncated, %d bytes total]", total)
}

// 去掉末尾不完整的 UTF-8 字符，截断时不会把一个汉字切成乱码。
func runePrefix(b []byte) []byte {
	for i := len(b) - 1; i >= 0 && i >= len(b)-utf8.UTFMax; i-- {
		if utf8.RuneStart(b[i]) {
			if !utf8.FullRune(b[i:]) {
				return b[:i]
			}
			return b
		}
	}
	return b
}
