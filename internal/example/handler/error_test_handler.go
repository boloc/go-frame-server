package handler

import (
	"errors"
	"net/http"

	"github.com/boloc/go-frame-server/pkg/response"
	"github.com/boloc/go-frame-server/pkg/throw"
	"github.com/gin-gonic/gin"
)

// ErrorTestHandler 错误测试处理器
type ErrorTestHandler struct{}

// NewErrorTestHandler 创建错误测试处理器
func NewErrorTestHandler() *ErrorTestHandler {
	return &ErrorTestHandler{}
}

// TestSuccess 测试成功响应
// GET /test/success
func (h *ErrorTestHandler) TestSuccess(c *gin.Context) {
	response.Success(c, gin.H{
		"user_id": 1,
		"name":    "张三",
	})
}

// TestApiError 测试 API 异常
// GET /test/error/api
func (h *ErrorTestHandler) TestApiError(c *gin.Context) {
	err := h.mockBusinessError()
	if err != nil {
		response.BusinessError(c, err)
		return
	}
	response.Success(c, nil)
}

// TestSqlError 测试 SQL 异常
// GET /test/error/sql
func (h *ErrorTestHandler) TestSqlError(c *gin.Context) {
	err := h.mockSqlError()
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, nil)
}

// TestValidationError 测试参数验证异常
// GET /test/error/validation
func (h *ErrorTestHandler) TestValidationError(c *gin.Context) {
	err := h.mockValidationError()
	if err != nil {
		response.ValidationError(c, err)
		return
	}
	response.Success(c, nil)
}

// TestClientError 测试客户端请求异常
// GET /test/error/client
func (h *ErrorTestHandler) TestClientError(c *gin.Context) {
	err := h.mockClientError()
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, nil)
}

// TestNotFoundError 测试未找到错误
// GET /test/error/notfound
func (h *ErrorTestHandler) TestNotFoundError(c *gin.Context) {
	response.NotFoundError(c, errors.New("用户不存在"))
}

// TestUnauthorizedError 测试未授权错误
// GET /test/error/unauthorized
func (h *ErrorTestHandler) TestUnauthorizedError(c *gin.Context) {
	response.UnauthorizedError(c)
}

// TestBadRequestError 测试错误请求
// GET /test/error/badrequest
func (h *ErrorTestHandler) TestBadRequestError(c *gin.Context) {
	response.BadRequestError(c, "参数 id 必须大于 0")
}

// TestPanicError 测试 Panic 错误
// GET /test/error/panic
func (h *ErrorTestHandler) TestPanicError(c *gin.Context) {
	panic("模拟 panic 错误")
}

// TestCustomResponse 测试自定义响应
// GET /test/custom
func (h *ErrorTestHandler) TestCustomResponse(c *gin.Context) {
	response.SuccessWithMessage(c, "操作成功", gin.H{
		"updated": true,
	})
}

// ==================== Mock 方法 ====================

// mockBusinessError 模拟业务错误
func (h *ErrorTestHandler) mockBusinessError() error {
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

// SimpleError 简单错误响应（不使用 response 包）
func (h *ErrorTestHandler) SimpleError(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"code":    500,
		"message": "simple error",
	})
}
