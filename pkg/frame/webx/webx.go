// Package webx 提供 HTTP 绑定与统一响应：Bind / Success / Fail。
//
// 绑定入口按参数来源区分：Bind（按 Content-Type 自动选）、BindQuery、BindJSON、BindURI，
// 以及给历史客户端兜底的 BindWithQueryFallback。四个正常入口都会在绑定成功后跑一次
// pkg/frame/validate 校验。
package webx

import (
	"errors"
	"net/http"
	"sync/atomic"

	"github.com/boloc/go-frame-server/v2/pkg/alert"
	"github.com/boloc/go-frame-server/v2/pkg/errs"
	"github.com/boloc/go-frame-server/v2/pkg/frame/reqctx"
	"github.com/boloc/go-frame-server/v2/pkg/frame/validate"
	"github.com/boloc/go-frame-server/v2/pkg/logger"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

var onFailHook atomic.Pointer[func(c *gin.Context, err *errs.Error)]

// SetOnFail 注册错误响应钩子，在写响应之前调用。传 nil 清空。
//
// 回调拿到的是整个 *gin.Context 而不只是路由：路由用 c.FullPath() 就有，而客户端 IP、
// UA、Content-Length 这些只有 Context 里才有——像「请求体超限」这类告警，不知道是谁在
// 打大包基本没有处置价值。
//
// 回调在响应写出之前同步执行，不要在里面做慢操作（发 webhook 请自己起 goroutine，
// 或者走 pkg/alert，它内部已经是异步 + 并发上限）。
func SetOnFail(fn func(c *gin.Context, err *errs.Error)) {
	if fn == nil {
		onFailHook.Store(nil)
		return
	}
	onFailHook.Store(&fn)
}

// IncludeCallerInResponse 为 true 时把 Caller 写入响应体，默认 false。
var IncludeCallerInResponse atomic.Bool

// BizCodeKey 是 gin.Context 里存放业务码的 key，供 AccessLog / HTTPMetrics 在写完响应后读取。
const BizCodeKey = "webx.biz_code"

// BizCode 读取本次请求已经写出的业务码。未走 Success/Fail 时 ok=false。
func BizCode(c *gin.Context) (errs.Code, bool) {
	v, ok := c.Get(BizCodeKey)
	if !ok {
		return 0, false
	}
	code, ok := v.(errs.Code)
	if !ok {
		return 0, false
	}
	return code, true
}

// Response 统一响应体。
type Response struct {
	Code    errs.Code         `json:"code"`
	Message string            `json:"message"`
	Data    any               `json:"data,omitempty"`
	Fields  map[string]string `json:"fields,omitempty"`
	Caller  string            `json:"caller,omitempty"`
}

// Success 成功响应，固定 HTTP 200 + Code=0。
func Success(c *gin.Context, data any) {
	c.Set(BizCodeKey, errs.CodeOK)
	c.JSON(http.StatusOK, Response{Code: errs.CodeOK, Message: "success", Data: data})
}

// Fail 返回 HTTP 200，业务码放在 body.code。需要真实 HTTP 状态码时用 FailWithStatus。
func Fail(c *gin.Context, err error) {
	if err == nil {
		Success(c, nil)
		return
	}
	respond(c, errs.From(err), http.StatusOK)
}

// FailWithStatus 与 Fail 相同，但 HTTP 状态码跟随 Code.HTTPStatus()。
func FailWithStatus(c *gin.Context, err error) {
	if err == nil {
		Success(c, nil)
		return
	}
	e := errs.From(err)
	respond(c, e, e.Code.HTTPStatus())
}

func respond(c *gin.Context, e *errs.Error, status int) {
	c.Set(BizCodeKey, e.Code)
	logFailure(c, e)

	if hook := onFailHook.Load(); hook != nil {
		(*hook)(c, e)
	}

	resp := Response{
		Code:    e.Code,
		Message: e.Message,
		Fields:  e.Fields,
	}
	if IncludeCallerInResponse.Load() {
		resp.Caller = e.Caller
	}
	c.JSON(status, resp)
}

func logFailure(c *gin.Context, e *errs.Error) {
	// RequestID 来自 reqctx.FromGin：没有经过 ContextMiddleware 的路由（比如手写的
	// pprof/debug 路由）这里会是空字符串，不会 panic，也不会影响其它字段。
	// 补这个字段是为了让并发请求交织在一起的日志能按 X-Request-Id 串联起来定位到具体某一次请求。
	fields := []zap.Field{
		zap.Int("code", int(e.Code)),
		zap.String("message", e.Message),
		zap.String("caller", e.Caller),
		zap.String("route", c.FullPath()),
		zap.String("method", c.Request.Method),
		zap.String("request_id", reqctx.FromGin(c).RequestID),
	}
	if e.Err != nil {
		fields = append(fields, zap.Error(e.Err))
	}

	if e.Code >= errs.CodeInternal {
		logger.Error("webx: request failed", fields...)
		alert.Notify(c.Request.Context(), alert.Event{
			Scope: "webx", Name: c.FullPath(), Message: e.Message, Err: e.Err,
			Fields: map[string]any{"code": int(e.Code), "route": c.FullPath(), "method": c.Request.Method},
		})
		return
	}
	logger.Warn("webx: request failed", fields...)
}

// Bind 按 Content-Type 绑定并做字段校验，失败返回 *errs.Error。
func Bind(c *gin.Context, req any) error {
	if err := c.ShouldBind(req); err != nil {
		return errs.Wrap(errs.CodeInvalidParams, err, "请求参数格式错误")
	}
	return runValidate(req)
}

func BindQuery(c *gin.Context, req any) error {
	if err := c.ShouldBindQuery(req); err != nil {
		return errs.Wrap(errs.CodeInvalidParams, err, "请求参数格式错误")
	}
	return runValidate(req)
}

func BindJSON(c *gin.Context, req any) error {
	if err := c.ShouldBindJSON(req); err != nil {
		return errs.Wrap(errs.CodeInvalidParams, err, "请求参数格式错误")
	}
	return runValidate(req)
}

func BindURI(c *gin.Context, req any) error {
	if err := c.ShouldBindUri(req); err != nil {
		return errs.Wrap(errs.CodeInvalidParams, err, "路径参数格式错误")
	}
	return runValidate(req)
}

// BindWithQueryFallback 与 Bind 相同，但 body 解析失败时再回退读一次 query string。
//
// 只给「POST 请求的 body 可能整个丢失」的历史客户端用。已知场景：部分手机浏览器
// （iOS 上的 QQ 浏览器是常见一例）在页面跳转时会把 POST body 丢掉，只剩 URL 上的 query。
// 这类客户端已经发出去了改不动，只能服务端两边都认。
//
// 注意 body 解析失败时 req 可能已经被填了一半，回退的 query 绑定只覆盖 query 里出现的
// 字段，残留值会保留——这与历史实现一致，也是这个兼容本身固有的模糊之处。
// 两次都失败时返回 body 的错误，它比 query 的错误更能说明客户端发了什么。
func BindWithQueryFallback(c *gin.Context, req any) error {
	if err := c.ShouldBind(req); err != nil {
		if fallbackErr := c.ShouldBindQuery(req); fallbackErr != nil {
			return errs.Wrap(errs.CodeInvalidParams, err, "请求参数格式错误")
		}
	}
	return runValidate(req)
}

func runValidate(req any) error {
	if fields := validate.Struct(req); fields != nil {
		return errs.InvalidParams("参数校验失败").WithFields(fields)
	}

	if v, ok := req.(validate.Validatable); ok {
		if err := v.Validate(); err != nil {
			if e, ok := errors.AsType[*errs.Error](err); ok {
				return e
			}
			return errs.InvalidParams(err.Error())
		}
	}
	return nil
}
