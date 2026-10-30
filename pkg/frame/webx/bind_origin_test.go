package webx

import (
	"errors"
	"net/http"
	"testing"

	"github.com/boloc/go-frame-server/v2/pkg/errs"
	"github.com/gin-gonic/gin"
)

type customValidateReq struct {
	Name string `json:"name"`
	err  error
}

func (r *customValidateReq) Validate() error { return r.err }

func newRegisterUserErr() error { return errs.InvalidParams("账号已被占用") }

// 模拟业务 handler：webx 构造的绑定/校验错误应指向这里调用 Bind 的那一行。
func registerHandler(c *gin.Context, req any) error { return BindJSON(c, req) }

func TestBindErrorsPointAtCaller(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name string
		body string
		req  any
		want string
	}{
		{name: "JSON 格式错误", body: `{`, req: &fallbackReq{}, want: "webx.registerHandler"},
		{name: "validate 标签不通过", body: `{}`, req: &fallbackReq{}, want: "webx.registerHandler"},
		{name: "Validate() 返回普通 error", body: `{}`, req: &customValidateReq{err: errors.New("name 不合法")}, want: "webx.registerHandler"},
		{name: "Validate() 返回业务 *errs.Error 原样保留", body: `{}`, req: &customValidateReq{err: newRegisterUserErr()}, want: "webx.newRegisterUserErr"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newBindContext(http.MethodPost, "/register", "application/json", tt.body)
			e := errs.From(registerHandler(c, tt.req))
			if e == nil || e.Func != tt.want {
				t.Fatalf("Func = %v, want %q (err=%v)", e.Func, tt.want, e)
			}
		})
	}
}
