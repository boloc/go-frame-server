package webx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/boloc/go-frame-server/v2/pkg/errs"
	"github.com/gin-gonic/gin"
)

type fallbackReq struct {
	NavID uint   `json:"nav_id" form:"nav_id" validate:"required"`
	Tag   string `json:"tag" form:"tag"`
}

// newBindContext 造一个带请求的 gin.Context，body 为空字符串时不带 body。
func newBindContext(method, target, contentType, body string) *gin.Context {
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, target, nil)
	} else {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req
	return c
}

func TestBindWithQueryFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name        string
		method      string
		target      string
		contentType string
		body        string
		wantNavID   uint
		wantTag     string
		wantCode    errs.Code // 0 表示期望成功
	}{
		{
			name:      "GET 读 query",
			method:    http.MethodGet,
			target:    "/nav-products?nav_id=7&tag=a",
			wantNavID: 7,
			wantTag:   "a",
		},
		{
			name:        "POST 正常 JSON body",
			method:      http.MethodPost,
			target:      "/nav-products",
			contentType: "application/json",
			body:        `{"nav_id":7,"tag":"a"}`,
			wantNavID:   7,
			wantTag:     "a",
		},
		{
			// 这条是这个函数存在的唯一理由：客户端把 POST body 丢了，参数只剩在 URL 上。
			name:        "POST body 丢失时回退 query",
			method:      http.MethodPost,
			target:      "/nav-products?nav_id=7&tag=a",
			contentType: "application/json",
			wantNavID:   7,
			wantTag:     "a",
		},
		{
			name:        "POST body 是坏 JSON 时回退 query",
			method:      http.MethodPost,
			target:      "/nav-products?nav_id=7",
			contentType: "application/json",
			body:        `{"nav_id":`,
			wantNavID:   7,
		},
		{
			name:        "POST 表单 body",
			method:      http.MethodPost,
			target:      "/nav-products",
			contentType: "application/x-www-form-urlencoded",
			body:        "nav_id=7",
			wantNavID:   7,
		},
		{
			// body 和 query 都给不出参数：不能悄悄放过，要报参数错误。
			name:        "POST body 坏且 query 为空",
			method:      http.MethodPost,
			target:      "/nav-products",
			contentType: "application/json",
			body:        `{"nav_id":`,
			wantCode:    errs.CodeInvalidParams,
		},
		{
			// 回退之后 validate tag 仍然要跑，不能因为走了兼容路径就跳过校验。
			name:        "回退后仍然执行 validate 校验",
			method:      http.MethodPost,
			target:      "/nav-products?tag=a",
			contentType: "application/json",
			wantCode:    errs.CodeInvalidParams,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newBindContext(tt.method, tt.target, tt.contentType, tt.body)

			var req fallbackReq
			err := BindWithQueryFallback(c, &req)

			if tt.wantCode != 0 {
				if err == nil {
					t.Fatalf("期望失败，实际成功，req = %+v", req)
				}
				if !errs.Is(err, tt.wantCode) {
					t.Fatalf("code = %d，期望 %d", errs.From(err).Code, tt.wantCode)
				}
				return
			}

			if err != nil {
				t.Fatalf("期望成功，实际 error = %v", err)
			}
			if req.NavID != tt.wantNavID {
				t.Fatalf("nav_id = %d，期望 %d", req.NavID, tt.wantNavID)
			}
			if req.Tag != tt.wantTag {
				t.Fatalf("tag = %q，期望 %q", req.Tag, tt.wantTag)
			}
		})
	}
}

// Bind 不做回退：POST body 丢失时必须失败，否则 BindWithQueryFallback 就没有存在意义了。
func TestBindDoesNotFallBackToQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)

	c := newBindContext(http.MethodPost, "/nav-products?nav_id=7", "application/json", "")
	var req fallbackReq
	if err := Bind(c, &req); err == nil {
		t.Fatalf("Bind 不应回退读 query，实际成功绑到 nav_id = %d", req.NavID)
	}
}
