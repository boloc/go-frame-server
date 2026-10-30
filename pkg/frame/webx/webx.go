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
//
// 失败日志 webx 已经记过（见 logFailure），回调里不要再把同一个错误写一遍日志，只做计数、
// 分流之类的旁路动作。
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
	respond(c, err, errs.FromCaller(err, 1), http.StatusOK)
}

// FailWithStatus 与 Fail 相同，但 HTTP 状态码跟随 Code.HTTPStatus()。
func FailWithStatus(c *gin.Context, err error) {
	if err == nil {
		Success(c, nil)
		return
	}
	e := errs.FromCaller(err, 1)
	respond(c, err, e, e.Code.HTTPStatus())
}

// err 是业务交进来的原始错误，e 是从中取出或兜底包装的 *errs.Error；响应用 e，日志用 err，
// 业务在外层 fmt.Errorf("...: %w") 补的上下文不会丢。
func respond(c *gin.Context, err error, e *errs.Error, status int) {
	c.Set(BizCodeKey, e.Code)
	logFailure(c, err, e)

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

// 错误挂到请求的日志作用域上，由 AccessLog 在请求结束时打成一条（级别按业务码）。
// 请求没有作用域（没装 AccessLog 或被它跳过的路由）时在这里自己记一条。5xxxx 同时 alert.Notify。
func logFailure(c *gin.Context, err error, e *errs.Error) {
	ctx := c.Request.Context()
	rc := reqctx.FromGin(c)
	route := c.FullPath()
	// 原始错误链上没有 *errs.Error（e 是兜底包装出来的，含 typed-nil *errs.Error）时记 e，才有 origin，
	// 也避免对 typed-nil 调 Error() 而 panic。
	logErr := err
	if ie, ok := errors.AsType[*errs.Error](err); !ok || ie == nil {
		logErr = e
	}

	if e.Code >= errs.CodeInternal {
		alert.Notify(ctx, alert.Event{
			Scope: "webx", Name: route, Message: e.Message, Err: e.Err,
			Fields: map[string]any{
				"code": int(e.Code), "route": route, "method": c.Request.Method,
				"request_id": rc.RequestID, "origin_func": e.Func, "origin": e.Caller,
			},
		})
	}

	if logger.AddToScope(ctx, logger.Err(logErr)) {
		return
	}
	fields := []logger.Field{
		logger.String("request_id", rc.RequestID),
		logger.String("method", c.Request.Method),
		logger.String("route", route),
		logger.Int("code", int(e.Code)),
		logger.Err(logErr),
	}
	if e.Code >= errs.CodeInternal {
		logger.Error("webx: request failed", fields...)
		return
	}
	logger.Warn("webx: request failed", fields...)
}

// Bind 按 Content-Type 绑定并做字段校验，失败返回 *errs.Error。
func Bind(c *gin.Context, req any) error {
	if err := c.ShouldBind(req); err != nil {
		return errs.Wrap(errs.CodeInvalidParams, err, "请求参数格式错误").AtCaller(1)
	}
	return runValidate(req)
}

func BindQuery(c *gin.Context, req any) error {
	if err := c.ShouldBindQuery(req); err != nil {
		return errs.Wrap(errs.CodeInvalidParams, err, "请求参数格式错误").AtCaller(1)
	}
	return runValidate(req)
}

func BindJSON(c *gin.Context, req any) error {
	if err := c.ShouldBindJSON(req); err != nil {
		return errs.Wrap(errs.CodeInvalidParams, err, "请求参数格式错误").AtCaller(1)
	}
	return runValidate(req)
}

func BindURI(c *gin.Context, req any) error {
	if err := c.ShouldBindUri(req); err != nil {
		return errs.Wrap(errs.CodeInvalidParams, err, "路径参数格式错误").AtCaller(1)
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
			return errs.Wrap(errs.CodeInvalidParams, err, "请求参数格式错误").AtCaller(1)
		}
	}
	return runValidate(req)
}

// 只能由 Bind* 直接调用：webx 构造的错误用 AtCaller(2) 指到调用 Bind* 的业务代码；
// 业务 Validate() 自己返回的 *errs.Error 原样透传，位置已经是业务的。
func runValidate(req any) error {
	if fields := validate.Struct(req); fields != nil {
		return errs.InvalidParams("参数校验失败").WithFields(fields).AtCaller(2)
	}

	if v, ok := req.(validate.Validatable); ok {
		if err := v.Validate(); err != nil {
			if e, ok := errors.AsType[*errs.Error](err); ok {
				return e
			}
			return errs.InvalidParams(err.Error()).AtCaller(2)
		}
	}
	return nil
}
