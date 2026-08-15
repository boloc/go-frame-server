// Package errs 提供统一的业务错误类型：显式错误码、可扩展映射，构造时记录 Caller。
package errs

import (
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
)

// Code 业务错误码。
type Code int

// 内置错误码。业务自定义码从 10000 起，避免与 0 / 4xxxx / 5xxxx 冲突。
const (
	CodeOK Code = 0

	CodeInvalidParams   Code = 40000 // 参数格式错误 / 校验未通过
	CodeUnauthorized    Code = 40100 // 未登录 / 凭证无效或过期
	CodeForbidden       Code = 40300 // 已登录但无权限
	CodeNotFound        Code = 40400 // 资源不存在
	CodeConflict        Code = 40900 // 状态冲突（重复提交、并发修改冲突等）
	CodeRequestTooLarge Code = 41300 // 请求体超过 MaxBodyBytes 限制
	CodeTooManyRequests Code = 42900 // 触发限流

	CodeInternal   Code = 50000 // 未分类的内部错误（兜底）
	CodeDatabase   Code = 50001 // 数据库操作失败
	CodeCache      Code = 50002 // 缓存（Redis 等）操作失败
	CodeDependency Code = 50003 // 依赖的外部服务返回错误
	CodeTimeout    Code = 50004 // 操作超时
)

var (
	httpStatusMap     atomic.Pointer[map[Code]int]
	defaultMessageMap atomic.Pointer[map[Code]string]
	writeMu           sync.Mutex
)

func init() {
	dm := map[Code]string{
		CodeOK:              "success",
		CodeInvalidParams:   "请求参数不合法",
		CodeUnauthorized:    "未登录或登录已过期",
		CodeForbidden:       "没有权限执行该操作",
		CodeNotFound:        "资源不存在",
		CodeConflict:        "操作冲突，请刷新后重试",
		CodeRequestTooLarge: "请求体过大",
		CodeTooManyRequests: "请求过于频繁，请稍后重试",
		CodeInternal:        "服务器内部错误",
		CodeDatabase:        "数据处理失败",
		CodeCache:           "缓存服务异常",
		CodeDependency:      "依赖的服务暂时不可用",
		CodeTimeout:         "请求超时",
	}
	defaultMessageMap.Store(&dm)

	hs := map[Code]int{
		CodeOK:              http.StatusOK,
		CodeInvalidParams:   http.StatusBadRequest,
		CodeUnauthorized:    http.StatusUnauthorized,
		CodeForbidden:       http.StatusForbidden,
		CodeNotFound:        http.StatusNotFound,
		CodeConflict:        http.StatusConflict,
		CodeRequestTooLarge: http.StatusRequestEntityTooLarge,
		CodeTooManyRequests: http.StatusTooManyRequests,
		CodeInternal:        http.StatusInternalServerError,
		CodeDatabase:        http.StatusInternalServerError,
		CodeCache:           http.StatusInternalServerError,
		CodeDependency:      http.StatusBadGateway,
		CodeTimeout:         http.StatusGatewayTimeout,
	}
	httpStatusMap.Store(&hs)
}

// HTTPStatus 返回该业务码建议对应的 HTTP 状态码；未登记的自定义码默认 500。
func (c Code) HTTPStatus() int {
	if m := httpStatusMap.Load(); m != nil {
		if s, ok := (*m)[c]; ok {
			return s
		}
	}
	return http.StatusInternalServerError
}

// RegisterHTTPStatus 为自定义 Code 登记 HTTP 状态码映射。建议在启动阶段调用。
func RegisterHTTPStatus(c Code, status int) {
	writeMu.Lock()
	defer writeMu.Unlock()

	old := *httpStatusMap.Load()
	next := make(map[Code]int, len(old)+1)
	for k, v := range old {
		next[k] = v
	}
	next[c] = status
	httpStatusMap.Store(&next)
}

// RegisterMessage 为自定义 Code 登记默认文案，配合 New(code, "") 使用。
func RegisterMessage(c Code, message string) {
	writeMu.Lock()
	defer writeMu.Unlock()

	old := *defaultMessageMap.Load()
	next := make(map[Code]string, len(old)+1)
	for k, v := range old {
		next[k] = v
	}
	next[c] = message
	defaultMessageMap.Store(&next)
}

// defaultMessageFor 无锁读取 defaultMessage 快照，New/Wrap 内部共用。
func defaultMessageFor(c Code) (string, bool) {
	m := defaultMessageMap.Load()
	if m == nil {
		return "", false
	}
	v, ok := (*m)[c]
	return v, ok
}

// Error 统一的业务错误。
//
//	Code    业务码
//	Message 展示文案
//	Err     底层原因，供日志和 errors.Is/As，默认不回给客户端
//	Fields  参数校验失败时的字段详情
//	Caller  首次构造时的业务代码位置
type Error struct {
	Code    Code
	Message string
	Err     error
	Fields  map[string]string
	Caller  string
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%d] %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("[%d] %s", e.Code, e.Message)
}

// Unwrap 支持 errors.Is / errors.As 穿透到底层原因。
func (e *Error) Unwrap() error { return e.Err }

const pkgImportPath = "github.com/boloc/go-frame-server/pkg/errs"

// captureCaller 返回调用栈上第一个不属于本包的帧。
func captureCaller() string {
	var pcs [16]uintptr
	n := runtime.Callers(1, pcs[:])
	if n == 0 {
		return ""
	}
	frames := runtime.CallersFrames(pcs[:n])
	for {
		frame, more := frames.Next()
		if !strings.HasPrefix(frame.Function, pkgImportPath+".") {
			return fmt.Sprintf("%s:%d", frame.File, frame.Line)
		}
		if !more {
			return ""
		}
	}
}

// New 创建一个业务错误。message 为空时使用该 Code 登记的默认文案。
func New(code Code, message string) *Error {
	if message == "" {
		message, _ = defaultMessageFor(code)
	}
	return &Error{Code: code, Message: message, Caller: captureCaller()}
}

// Newf 格式化创建，等价于 New(code, fmt.Sprintf(format, args...))。
func Newf(code Code, format string, args ...any) *Error {
	return New(code, fmt.Sprintf(format, args...))
}

// Wrap 把底层 error 包装为业务错误。err 为 nil 时返回 nil，调用方应先判断 err。
func Wrap(code Code, err error, message string) *Error {
	if err == nil {
		return nil
	}
	if message == "" {
		if m, ok := defaultMessageFor(code); ok {
			message = m
		} else {
			message = err.Error()
		}
	}
	return &Error{Code: code, Message: message, Err: err, Caller: captureCaller()}
}

// WithFields 附加字段级详情，典型场景是参数校验失败时标出具体字段。
func (e *Error) WithFields(fields map[string]string) *Error {
	e.Fields = fields
	return e
}

func InvalidParams(message string) *Error { return New(CodeInvalidParams, message) }
func Unauthorized(message string) *Error  { return New(CodeUnauthorized, message) }
func Forbidden(message string) *Error     { return New(CodeForbidden, message) }
func NotFound(message string) *Error      { return New(CodeNotFound, message) }
func Conflict(message string) *Error      { return New(CodeConflict, message) }

// RequestTooLarge 请求体超过 middleware.MaxBodyBytes 限制时构造。
func RequestTooLarge(message string) *Error { return New(CodeRequestTooLarge, message) }

func Internal(err error) *Error { return Wrap(CodeInternal, err, "") }

func Database(err error) *Error { return Wrap(CodeDatabase, err, "") }

// Cache 包装缓存（Redis 等）操作失败。
func Cache(err error) *Error { return Wrap(CodeCache, err, "") }

// Dependency 包装调用外部服务失败。
func Dependency(err error) *Error { return Wrap(CodeDependency, err, "") }

// Timeout 包装超时错误。
func Timeout(err error) *Error { return Wrap(CodeTimeout, err, "") }

// Is 判断 err 是否为指定 Code 的业务错误。
func Is(err error, code Code) bool {
	e := asError(err)
	return e != nil && e.Code == code
}

// From 从 error 中提取 *Error；提取不到时归为 CodeInternal。
//
// 对 typed-nil *Error（`var e *Error; return e` 这种 e 从未被赋值、但作为 error 接口
// 返回的畸形值，err == nil 判断不出来）故意不当成"没有错误"处理，而是仍然兜底包成
// CodeInternal 返回——见 errs_test.go 的 TestFromHandlesTypedNilWithoutPanicking：
// 这是故意选择"让这类 bug 大声地表现成一个 500"，而不是悄悄当成成功放过去，
// 避免真正的 bug（该赋值的分支忘了赋值）被这里的"宽容"掩盖掉。
func From(err error) *Error {
	if err == nil {
		return nil
	}
	if e := asError(err); e != nil {
		return e
	}
	return Wrap(CodeInternal, err, "")
}

func asError(err error) *Error {
	var e *Error
	if errors.As(err, &e) && e != nil {
		return e
	}
	return nil
}
