package handler

import (
	"errors"
	"net/http"

	"github.com/boloc/go-frame-server/pkg/throw"
	"github.com/boloc/go-frame-server/pkg/throw/handler"
	"github.com/gin-gonic/gin"
)

// ErrorTestHandler 错误测试处理器
type ErrorTestHandler struct{}

// NewErrorTestHandler 创建错误测试处理器
func NewErrorTestHandler() *ErrorTestHandler {
	return &ErrorTestHandler{}
}

// TestApiError 测试 API 异常
// GET /test/error/api
func (h *ErrorTestHandler) TestApiError(c *gin.Context) {
	// 模拟业务层调用
	err := h.mockBusinessError()
	if err != nil {
		h.handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success"})
}

// TestSqlError 测试 SQL 异常
// GET /test/error/sql
func (h *ErrorTestHandler) TestSqlError(c *gin.Context) {
	err := h.mockSqlError()
	if err != nil {
		h.handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success"})
}

// TestValidationError 测试参数验证异常
// GET /test/error/validation
func (h *ErrorTestHandler) TestValidationError(c *gin.Context) {
	err := h.mockValidationError()
	if err != nil {
		h.handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success"})
}

// TestClientError 测试客户端请求异常
// GET /test/error/client
func (h *ErrorTestHandler) TestClientError(c *gin.Context) {
	err := h.mockClientError()
	if err != nil {
		h.handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success"})
}

// mockBusinessError 模拟业务错误
func (h *ErrorTestHandler) mockBusinessError() error {
	// 这里模拟业务代码中发生错误
	return throw.ApiException(errors.New("用户不存在"))
}

// mockSqlError 模拟 SQL 错误
func (h *ErrorTestHandler) mockSqlError() error {
	return throw.SqlException(errors.New("record not found"))
}

// mockValidationError 模拟参数验证错误
func (h *ErrorTestHandler) mockValidationError() error {
	return throw.ValidationException(errors.New("参数 id 不能为空"))
}

// mockClientError 模拟客户端请求错误
func (h *ErrorTestHandler) mockClientError() error {
	return throw.ClientException(errors.New("connection refused"))
}

// handleError 统一错误处理
func (h *ErrorTestHandler) handleError(c *gin.Context, err error) {
	// 尝试获取自定义错误信息
	if customErr, ok := err.(interface{ Error() string }); ok {
		// 获取详细错误信息
		var errorPath, function string
		var code int

		switch e := err.(type) {
		case *throw.ApiError:
			code = e.Code
			errorPath = e.ErrorPath
			function = e.Function
		case *throw.SqlError:
			code = e.Code
			errorPath = e.ErrorPath
			function = e.Function
		case *throw.ValidationError:
			code = e.Code
			errorPath = e.ErrorPath
			function = e.Function
		case *throw.ClientError:
			code = e.Code
			errorPath = e.ErrorPath
			function = e.Function
		case *handler.ExceptionError:
			code = e.Code
			errorPath = e.ErrorPath
			function = e.Function
		default:
			code = 500
		}

		c.JSON(http.StatusOK, gin.H{
			"code":       code,
			"message":    customErr.Error(),
			"error_path": errorPath, // 这里应该显示调用 throw.XxxException 的业务代码位置
			"function":   function,
		})
		return
	}

	c.JSON(http.StatusInternalServerError, gin.H{
		"code":    500,
		"message": err.Error(),
	})
}
