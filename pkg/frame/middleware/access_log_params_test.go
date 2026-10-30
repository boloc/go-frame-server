package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/boloc/go-frame-server/v2/pkg/logger"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestParamMasker(t *testing.T) {
	t.Parallel()
	m := newParamMasker([]string{"secret"})

	tests := []struct {
		name string
		got  string
		want string
	}{
		{
			name: "JSON 嵌套字段脱敏，不区分大小写，中文原样",
			got:  m.json([]byte(`{"name":"小明","App_Secret":"s1","items":[{"id":1,"secret":"s2"}]}`)),
			want: `{"App_Secret":"***","items":[{"id":1,"secret":"***"}],"name":"小明"}`,
		},
		{
			name: "字段名命中但值是容器时往里找，保留结构",
			got:  m.json([]byte(`{"secrets":[{"id":1,"secret":"s"}]}`)),
			want: `{"secrets":[{"id":1,"secret":"***"}]}`,
		},
		{
			name: "截断的 JSON 按文本脱敏，值被截一半也不漏",
			got:  m.json([]byte(`{"name":"abc","secret":"s1s2s`)),
			want: `{"name":"abc","secret":"***"`,
		},
		{
			name: "截断的 JSON 里容器值保留结构",
			got:  m.json([]byte(`{"secrets":[{"id":1,"secret":"s"},{"id":2,"sec`)),
			want: `{"secrets":[{"id":1,"secret":"***"},{"id":2,"sec`,
		},
		{
			name: "非字符串值也脱敏",
			got:  m.json([]byte(`{"secret":123456,"x":1`)),
			want: `{"secret":"***","x":1`,
		},
		{
			name: "表单脱敏，保持原顺序",
			got:  m.form("name=abc&client_secret=s%40s&x=1"),
			want: "name=abc&client_secret=***&x=1",
		},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s:\n got  %s\n want %s", tt.name, tt.got, tt.want)
		}
	}
}

func TestParamMaskerWithoutKeysKeepsInput(t *testing.T) {
	t.Parallel()
	m := newParamMasker(nil)
	if got := m.json([]byte(`{"secret":"s"}`)); got != `{"secret":"s"}` {
		t.Errorf("json = %s", got)
	}
	if got := m.form("secret=s"); got != "secret=s" {
		t.Errorf("form = %s", got)
	}
}

func serveAccess(t *testing.T, opts []AccessLogOption, req *http.Request) (map[string]any, string) {
	t.Helper()
	core, logs := observer.New(zapcore.DebugLevel)
	t.Cleanup(logger.Replace(zap.New(core)))

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(AccessLog(opts...))
	var handlerSaw string
	r.Any("/p", func(c *gin.Context) {
		b, _ := io.ReadAll(c.Request.Body)
		handlerSaw = string(b)
		c.Status(http.StatusOK)
	})
	r.ServeHTTP(httptest.NewRecorder(), req)

	entries := logs.FilterMessage("http: access").All()
	if len(entries) != 1 {
		t.Fatalf("want one access log, got %d", len(entries))
	}
	return entries[0].ContextMap(), handlerSaw
}

func TestAccessLogRecordsQueryAndBody(t *testing.T) {
	body := `{"name":"abc","secret":"s1"}`
	req := httptest.NewRequest(http.MethodPost, "/p?from=app&secret=q1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	fields, handlerSaw := serveAccess(t, []AccessLogOption{WithSensitiveKeys("secret")}, req)
	if handlerSaw != body {
		t.Fatalf("handler 读到的 body 被改了: %q", handlerSaw)
	}
	if fields["query"] != "from=app&secret=***" {
		t.Errorf("query = %v", fields["query"])
	}
	if fields["body"] != `{"name":"abc","secret":"***"}` {
		t.Errorf("body = %v", fields["body"])
	}
	if _, ok := fields["body_truncated"]; ok {
		t.Error("未超长不应标记截断")
	}
}

func TestAccessLogDefaultDoesNotMask(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/p?secret=q1", strings.NewReader("secret=b1"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	fields, _ := serveAccess(t, nil, req)
	if fields["query"] != "secret=q1" || fields["body"] != "secret=b1" {
		t.Errorf("未指定 WithSensitiveKeys 时应原样记录，got query=%v body=%v", fields["query"], fields["body"])
	}
}

func TestAccessLogTruncatesHugeParams(t *testing.T) {
	huge := strings.Repeat("a", 10000)
	req := httptest.NewRequest(http.MethodPost, "/p?q="+huge, strings.NewReader(`{"name":"`+huge+`"}`))
	req.Header.Set("Content-Type", "application/json")

	fields, handlerSaw := serveAccess(t, []AccessLogOption{WithParamMaxBytes(64)}, req)
	if len(handlerSaw) != len(huge)+11 {
		t.Fatalf("handler 应读到完整 body，got %d bytes", len(handlerSaw))
	}
	wantQuery := "q=" + strings.Repeat("a", 62) + "…[truncated, 10002 bytes total]"
	if fields["query"] != wantQuery || fields["query_truncated"] != true {
		t.Errorf("query = %v, truncated=%v", fields["query"], fields["query_truncated"])
	}
	wantBody := `{"name":"` + strings.Repeat("a", 55) + "…[truncated, 10011 bytes total]"
	if fields["body"] != wantBody || fields["body_truncated"] != true {
		t.Errorf("body = %v, truncated=%v", fields["body"], fields["body_truncated"])
	}
}

func TestAccessLogTruncatesOnRuneBoundary(t *testing.T) {
	// "中" 是 3 字节，{"n":" 是 6 字节；上限 10 时第 2 个汉字只截到 1 个字节，应整个丢掉。
	req := httptest.NewRequest(http.MethodPost, "/p", strings.NewReader(`{"n":"中中中"}`))
	req.Header.Set("Content-Type", "application/json")

	fields, _ := serveAccess(t, []AccessLogOption{WithParamMaxBytes(10)}, req)
	if want := `{"n":"中…[truncated, 17 bytes total]`; fields["body"] != want {
		t.Errorf("body = %v, want %v", fields["body"], want)
	}
}

func TestAccessLogSkipsBinaryBodyAndCanDisable(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/p", strings.NewReader("--boundary\r\nfile-bytes"))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=boundary")
	fields, _ := serveAccess(t, nil, req)
	if _, ok := fields["body"]; ok {
		t.Errorf("文件上传不应记 body: %v", fields["body"])
	}

	req = httptest.NewRequest(http.MethodPost, "/p?a=1", strings.NewReader(`{"a":1}`))
	req.Header.Set("Content-Type", "application/json")
	fields, _ = serveAccess(t, []AccessLogOption{WithParamMaxBytes(0)}, req)
	if _, ok := fields["query"]; ok {
		t.Error("WithParamMaxBytes(0) 应关闭 query 记录")
	}
	if _, ok := fields["body"]; ok {
		t.Error("WithParamMaxBytes(0) 应关闭 body 记录")
	}
}
