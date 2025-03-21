package response

import (
	"fmt"
	"frame-server/pkg/frame/config"
	"frame-server/pkg/logger"
	"frame-server/pkg/requests"
	"frame-server/pkg/throw"
	"frame-server/pkg/throw/enum"
	"frame-server/pkg/throw/handler"
	"net/http"

	"runtime"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-sql-driver/mysql"
	"go.uber.org/zap"
)

// Response 统一响应结构
type Response struct {
	Code        int                `json:"code"`
	Message     string             `json:"message"`
	ErrorPath   string             `json:"error_path,omitempty"`
	Function    string             `json:"error_function,omitempty"`
	Data        any                `json:"data,omitempty"`
	RequestInfo *requests.Requests `json:"request_info,omitempty"`
	Extra       any                `json:"extra,omitempty"`
}

// Stackconfig 堆栈帧结构
type Stackconfig struct {
	Function string `json:"function"`  // 函数名
	File     string `json:"file"`      // 文件名
	Line     int    `json:"line"`      // 行号
	FullPath string `json:"full_path"` // 完整路径
}

// Success 成功响应
func Success(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Response{
		Code:    enum.SUCCESS,
		Message: enum.GetMessage(enum.SUCCESS),
		Data:    data,
	})
}

// 业务错误
func BusinessError(c *gin.Context, err error) {
	defaultErrPath, defaultErrFunction := handler.ErrorCaller()
	errorHandle(c, err, defaultErrPath, defaultErrFunction)
}

// 参数验证错误
func ValidationError(c *gin.Context, err error) {
	defaultErrPath, defaultErrFunction := handler.ErrorCaller()
	errorHandle(c, err, defaultErrPath, defaultErrFunction)
}

// 未找到
func NotFoundError(c *gin.Context, err error) {
	errPath, errFunction := handler.ErrorCaller()
	errorHandle(c, throw.ApiCustomException(enum.NOT_FOUND, err.Error()), errPath, errFunction)
}

// 服务器内部错误
func ServerError(c *gin.Context, err error) {
	errPath, errFunction := handler.ErrorCaller()
	errorHandle(c, throw.ApiCustomException(enum.SERVER_ERROR, err.Error()), errPath, errFunction)
}

// 未授权
func UnauthorizedError(c *gin.Context) {
	errPath, errFunction := handler.ErrorCaller()
	errorHandle(c, throw.ApiCustomException(enum.UNAUTHORIZED, enum.GetMessage(enum.UNAUTHORIZED)), errPath, errFunction)
}

// 禁止访问
func ForbiddenError(c *gin.Context, err error) {
	errPath, errFunction := handler.ErrorCaller()
	errorHandle(c, throw.ApiCustomException(enum.FORBIDDEN, err.Error()), errPath, errFunction)
}

// 方法不允许
func MethodNotAllowedError(c *gin.Context, err error) {
	errPath, errFunction := handler.ErrorCaller()
	errorHandle(c, throw.ApiCustomException(enum.METHOD_NOT_ALLOWED, err.Error()), errPath, errFunction)
}

// 请求过多
func TooManyRequests(c *gin.Context, err error) {
	errPath, errFunction := handler.ErrorCaller()
	errorHandle(c, throw.ApiCustomException(enum.TOO_MANY_REQUESTS, err.Error()), errPath, errFunction)
}

// 格式化堆栈信息
func formatStackTrace(stack string) []Stackconfig {
	lines := strings.Split(stack, "\n")
	var configs []Stackconfig

	for i := 0; i < len(lines)-1; i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		// 如果是函数调用行
		if strings.HasPrefix(line, "frame-server/") || strings.HasPrefix(line, "github.com/") {
			config := Stackconfig{
				Function: line,
			}

			// 检查下一行是否包含文件信息
			if i+1 < len(lines) && strings.Contains(lines[i+1], ".go:") {
				fileLine := strings.TrimSpace(lines[i+1])
				config.FullPath = fileLine

				// 提取文件名和行号
				if idx := strings.LastIndex(fileLine, "/"); idx != -1 {
					config.File = fileLine[idx+1:]
					// 提取行号
					if parts := strings.Split(config.File, ":"); len(parts) == 2 {
						config.File = parts[0]
						config.Line, _ = strconv.Atoi(strings.Split(parts[1], " ")[0])
					}
				}

				configs = append(configs, config)
				i++ // 跳过文件行
			}
		}
	}

	// 反转调用栈，使最近的调用在最上面
	for i := 0; i < len(configs)/2; i++ {
		configs[i], configs[len(configs)-1-i] = configs[len(configs)-1-i], configs[i]
	}

	return configs
}

// Error Panic 错误响应
func ErrorPanic(c *gin.Context, err any) {
	// 获取堆栈跟踪信息
	stackTrace := make([]byte, 4096)
	n := runtime.Stack(stackTrace, false)
	stackInfo := string(stackTrace[:n])

	// 格式化堆栈信息
	stackconfigs := formatStackTrace(stackInfo)

	// 转换 err 为 error 类型
	var errMsg error
	switch e := err.(type) {
	case error:
		errMsg = e
	default:
		errMsg = fmt.Errorf("%v", e)
	}

	result := Response{
		Code:    enum.SERVER_ERROR,
		Message: fmt.Sprintf("panic error: %v", errMsg),
		RequestInfo: &requests.Requests{
			RequestIP:     c.ClientIP(),
			RequestMethod: c.Request.Method,
			RequestPath:   c.Request.URL.Path,
			RequestHeader: &c.Request.Header,
		},
		Extra: map[string]interface{}{
			"stack_configs": stackconfigs,
			// "raw_stack":    stackInfo, // 保留原始堆栈信息，以备需要
		},
	}
	// 在协程之前复制 result的值，避免被后续修改影响
	resultCopy := result
	go func() {
		requestPath := resultCopy.RequestInfo.RequestPath
		requestMethod := resultCopy.RequestInfo.RequestMethod
		requestIP := resultCopy.RequestInfo.RequestIP

		// 记录详细的错误日志
		logger.Error("Panic 错误",
			zap.String("error", errMsg.Error()),
			zap.String("request_ip", requestIP),
			zap.String("request_path", requestPath),
			zap.String("request_method", requestMethod),
		)

	}()

	// 非生产环境返回一些debug信息
	if !config.IsProduction() {
		// 重新赋值一个Response,不用引用类型
		result = Response{
			Code:        enum.SERVER_ERROR,
			Message:     enum.GetMessage(enum.SERVER_ERROR),
			Extra:       nil,
			RequestInfo: nil,
		}
	}
	c.JSON(http.StatusOK, result)
}

// 错误处理
func errorHandle(c *gin.Context, err error, errPath, errFunction string) {
	var response *Response

	var errCode int
	var errMsg string

	// 使用反射获取错误的具体类型
	switch errType := err.(type) {

	// 常规api错误
	case *throw.ApiError:
		errCode = errType.Code
		errMsg = errType.ErrorMsg
		errPath, errFunction = errType.ErrorPath, errType.Function
	// 自定义api错误
	case *throw.ApiCustomError:
		errCode = errType.Code
		errMsg = errType.ErrorMsg
		errPath, errFunction = chooseErrorDetails(errPath, errFunction, errType.ErrorPath, errType.Function)

	// 参数验证错误
	case *throw.ValidationError:
		errCode = errType.Code
		errMsg = errType.ErrorMsg
		errPath, errFunction = chooseErrorDetails(errPath, errFunction, errType.ErrorPath, errType.Function)
		// response.Extra = map[string]any{"error_fields": errType.ValidationErrors}

	// 请求客户端错误
	case *throw.ClientError:
		errCode = errType.Code
		errMsg = errType.ErrorMsg
		errPath, errFunction = errType.ErrorPath, errType.Function

	// 自主sql错误
	case *throw.SqlError:
		errCode = errType.Code
		errMsg = errType.ErrorMsg
		if config.IsProduction() {
			errMsg = "Sql Error"
		}
		errPath, errFunction = errType.ErrorPath, errType.Function

	// 系统内mysql错误
	case *mysql.MySQLError:
		errCode = enum.SQL_ERROR
		errMsg = fmt.Sprintf("[%d] %s", errType.Number, errType.Message)
		if config.IsProduction() {
			errMsg = "Data query errors"
		}

	// 处理其他类型的错误
	default:
		errCode = enum.SERVICE_UNAVAILABLE
		errMsg = err.Error()
		// 默认都为服务错误
		if config.IsProduction() {
			errMsg = enum.GetMessage(enum.SERVICE_UNAVAILABLE)
		}
	}

	response = buildErrorResponse(c, errCode, errMsg, errPath, errFunction)
	c.JSON(http.StatusOK, &response)
}

// 实现逻辑:如果传入的错误路径和函数名不为空，则覆盖默认的错误路径和函数名
func chooseErrorDetails(errPath, errFunction, errTypePath, errTypeFunction string) (string, string) {
	if errPath != "" {
		errTypePath = errPath
	}
	if errFunction != "" {
		errTypeFunction = errFunction
	}
	return errTypePath, errTypeFunction
}

// 组装错误响应
func buildErrorResponse(c *gin.Context, code int, message, errorPath, function string) *Response {
	result := &Response{
		Code:    code,
		Message: message,
	}
	// 获取request
	requests := c.MustGet("requests").(*requests.Requests)

	go func() {
		// 记录日志
		logger.Error("响应返回错误记录",
			zap.Int("error_code", result.Code),
			zap.String("error_msg", result.Message),
			zap.String("error_path", result.ErrorPath),
			zap.String("error_function", result.Function),
			// zap.Any("request_info", result.RequestInfo),
		)
	}()

	// 非生产环境返回的debug消息
	if !config.IsProduction() {
		if errorPath != "" {
			result.ErrorPath = errorPath
		}
		if function != "" {
			result.Function = function
		}

		// 输出request
		result.RequestInfo = requests
	}

	return result
}
