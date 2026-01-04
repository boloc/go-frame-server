package response

import (
	"fmt"
	"net/http"
	"runtime"
	"strconv"
	"strings"

	"github.com/boloc/go-frame-server/pkg/frame/config"
	"github.com/boloc/go-frame-server/pkg/logger"
	"github.com/boloc/go-frame-server/pkg/throw"
	"github.com/boloc/go-frame-server/pkg/throw/enum"
	"github.com/boloc/go-frame-server/pkg/throw/handler"
	"github.com/gin-gonic/gin"
	"github.com/go-sql-driver/mysql"
	"go.uber.org/zap"
)

// Response 统一响应结构
type Response struct {
	Code      int    `json:"code"`                     // 业务状态码
	Message   string `json:"message"`                  // 响应消息
	Data      any    `json:"data,omitempty"`           // 响应数据
	ErrorPath string `json:"error_path,omitempty"`     // 错误路径（仅开发环境）
	Function  string `json:"error_function,omitempty"` // 错误函数（仅开发环境）
	Extra     any    `json:"extra,omitempty"`          // 额外信息
}

// StackFrame 堆栈帧结构（用于 panic 错误追踪）
type StackFrame struct {
	Function string `json:"function"`  // 函数名
	File     string `json:"file"`      // 文件名
	Line     int    `json:"line"`      // 行号
	FullPath string `json:"full_path"` // 完整路径
}

// ==================== 成功响应 ====================

// Success 成功响应
func Success(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Response{
		Code:    enum.SUCCESS,
		Message: enum.GetMessage(enum.SUCCESS),
		Data:    data,
	})
}

// SuccessWithMessage 成功响应（自定义消息）
func SuccessWithMessage(c *gin.Context, message string, data any) {
	c.JSON(http.StatusOK, Response{
		Code:    enum.SUCCESS,
		Message: message,
		Data:    data,
	})
}

// ==================== 错误响应 ====================

// Error 通用错误响应（自动追溯调用位置）
func Error(c *gin.Context, err error) {
	errPath, errFunction := getCallerInfo(2)
	handleError(c, err, errPath, errFunction)
}

// BusinessError 业务错误
func BusinessError(c *gin.Context, err error) {
	errPath, errFunction := getCallerInfo(2)
	handleError(c, err, errPath, errFunction)
}

// ValidationError 参数验证错误
func ValidationError(c *gin.Context, err error) {
	errPath, errFunction := getCallerInfo(2)
	handleError(c, err, errPath, errFunction)
}

// NotFoundError 未找到
func NotFoundError(c *gin.Context, err error) {
	errPath, errFunction := getCallerInfo(2)
	handleError(c, throw.ApiCustomException(enum.NOT_FOUND, err.Error()), errPath, errFunction)
}

// ServerError 服务器内部错误
func ServerError(c *gin.Context, err error) {
	errPath, errFunction := getCallerInfo(2)
	handleError(c, throw.ApiCustomException(enum.SERVER_ERROR, err.Error()), errPath, errFunction)
}

// UnauthorizedError 未授权
func UnauthorizedError(c *gin.Context) {
	errPath, errFunction := getCallerInfo(2)
	handleError(c, throw.ApiCustomException(enum.UNAUTHORIZED, enum.GetMessage(enum.UNAUTHORIZED)), errPath, errFunction)
}

// ForbiddenError 禁止访问
func ForbiddenError(c *gin.Context, err error) {
	errPath, errFunction := getCallerInfo(2)
	handleError(c, throw.ApiCustomException(enum.FORBIDDEN, err.Error()), errPath, errFunction)
}

// MethodNotAllowedError 方法不允许
func MethodNotAllowedError(c *gin.Context, err error) {
	errPath, errFunction := getCallerInfo(2)
	handleError(c, throw.ApiCustomException(enum.METHOD_NOT_ALLOWED, err.Error()), errPath, errFunction)
}

// TooManyRequestsError 请求过多
func TooManyRequestsError(c *gin.Context, err error) {
	errPath, errFunction := getCallerInfo(2)
	handleError(c, throw.ApiCustomException(enum.TOO_MANY_REQUESTS, err.Error()), errPath, errFunction)
}

// BadRequestError 错误请求
func BadRequestError(c *gin.Context, message string) {
	errPath, errFunction := getCallerInfo(2)
	handleError(c, throw.ApiCustomException(enum.BAD_REQUEST, message), errPath, errFunction)
}

// ==================== Panic 错误响应 ====================

// PanicError Panic 错误响应（用于 recovery 中间件）
func PanicError(c *gin.Context, err any) {
	// 获取堆栈跟踪信息
	stackTrace := make([]byte, 4096)
	n := runtime.Stack(stackTrace, false)
	stackInfo := string(stackTrace[:n])

	// 格式化堆栈信息
	stackFrames := formatStackTrace(stackInfo)

	// 转换 err 为 error 类型
	var errMsg string
	switch e := err.(type) {
	case error:
		errMsg = e.Error()
	default:
		errMsg = fmt.Sprintf("%v", e)
	}

	// 记录日志
	logger.Error("Panic recovered",
		zap.String("error", errMsg),
		zap.String("stack", stackInfo),
	)

	result := Response{
		Code:    enum.SERVER_ERROR,
		Message: fmt.Sprintf("panic error: %s", errMsg),
	}

	// 非生产环境返回堆栈信息
	if !isProduction() {
		result.Extra = map[string]any{
			"stack_frames": stackFrames,
		}
	} else {
		result.Message = enum.GetMessage(enum.SERVER_ERROR)
	}

	c.JSON(http.StatusInternalServerError, result)
}

// ==================== 内部函数 ====================

// handleError 统一错误处理
func handleError(c *gin.Context, err error, defaultErrPath, defaultErrFunction string) {
	var errCode int
	var errMsg string
	var errPath, errFunction string
	var extra any

	// 根据错误类型提取信息
	switch e := err.(type) {
	case *throw.ApiError:
		errCode = e.Code
		errMsg = e.ErrorMsg
		errPath = e.ErrorPath
		errFunction = e.Function

	case *throw.ApiCustomError:
		errCode = e.Code
		errMsg = e.ErrorMsg
		errPath = chooseValue(e.ErrorPath, defaultErrPath)
		errFunction = chooseValue(e.Function, defaultErrFunction)

	case *throw.ValidationError:
		errCode = e.Code
		errMsg = e.ErrorMsg
		errPath = chooseValue(e.ErrorPath, defaultErrPath)
		errFunction = chooseValue(e.Function, defaultErrFunction)
		if len(e.ValidationErrors) > 0 {
			extra = map[string]any{"fields": e.ValidationErrors}
		}

	case *throw.ClientError:
		errCode = e.Code
		errMsg = e.ErrorMsg
		errPath = e.ErrorPath
		errFunction = e.Function

	case *throw.SqlError:
		errCode = e.Code
		errMsg = e.ErrorMsg
		errPath = e.ErrorPath
		errFunction = e.Function

	case *mysql.MySQLError:
		errCode = enum.SQL_ERROR
		errMsg = fmt.Sprintf("[%d] %s", e.Number, e.Message)
		errPath = defaultErrPath
		errFunction = defaultErrFunction

	case *handler.ExceptionError:
		errCode = e.Code
		errMsg = e.ErrorMsg
		errPath = e.ErrorPath
		errFunction = e.Function

	default:
		errCode = enum.SERVER_ERROR
		errMsg = err.Error()
		errPath = defaultErrPath
		errFunction = defaultErrFunction
	}

	// 记录错误日志
	logger.Error("Response error",
		zap.Int("code", errCode),
		zap.String("message", errMsg),
		zap.String("path", errPath),
		zap.String("function", errFunction),
		zap.String("request_uri", c.Request.RequestURI),
		zap.String("method", c.Request.Method),
	)

	// 构建响应
	response := Response{
		Code:    errCode,
		Message: errMsg,
		Extra:   extra,
	}

	// 非生产环境返回调试信息
	if !isProduction() {
		response.ErrorPath = errPath
		response.Function = errFunction
	}

	c.JSON(http.StatusOK, response)
}

// getCallerInfo 获取调用者信息
func getCallerInfo(skip int) (string, string) {
	pc, file, line, ok := runtime.Caller(skip)
	if !ok {
		return "", ""
	}
	return fmt.Sprintf("%s:%d", file, line), runtime.FuncForPC(pc).Name()
}

// chooseValue 选择非空值
func chooseValue(primary, fallback string) string {
	if primary != "" {
		return primary
	}
	return fallback
}

// isProduction 判断是否是生产环境
func isProduction() bool {
	return config.IsProduction()
}

// formatStackTrace 格式化堆栈信息
func formatStackTrace(stack string) []StackFrame {
	lines := strings.Split(stack, "\n")
	var frames []StackFrame

	for i := 0; i < len(lines)-1; i += 2 {
		funcLine := strings.TrimSpace(lines[i])
		if funcLine == "" || i+1 >= len(lines) {
			continue
		}

		fileLine := strings.TrimSpace(lines[i+1])
		if !strings.Contains(fileLine, ".go:") {
			continue
		}

		frame := StackFrame{
			Function: funcLine,
			FullPath: fileLine,
		}

		// 提取文件名和行号
		if idx := strings.LastIndex(fileLine, "/"); idx != -1 {
			fileInfo := fileLine[idx+1:]
			if parts := strings.Split(fileInfo, ":"); len(parts) >= 2 {
				frame.File = parts[0]
				lineNum := strings.Split(parts[1], " ")[0]
				frame.Line, _ = strconv.Atoi(lineNum)
			}
		}

		frames = append(frames, frame)
	}

	return frames
}
