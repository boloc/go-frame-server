package throw

import (
	"go-frame-server/pkg/throw/enum"
	"go-frame-server/pkg/throw/handler"
)

type ApiError struct {
	*handler.ExceptionError
}

type ApiCustomError struct {
	*handler.ExceptionError
}

// API请求异常
func ApiException(err error) error {
	if err == nil {
		return nil
	}

	// 获取错误路径和函数名
	ErrorPath, Function := handler.ErrorCaller()

	return &ApiError{
		ExceptionError: &handler.ExceptionError{
			Code:      enum.BAD_REQUEST,
			ErrorMsg:  err.Error(),
			ErrorPath: ErrorPath,
			Function:  Function,
		},
	}
}

// API请求异常
func ApiCustomException(code int, msg string) error {
	// 获取错误路径和函数名
	ErrorPath, Function := handler.ErrorCaller()

	return &ApiCustomError{
		ExceptionError: &handler.ExceptionError{
			Code:      code,
			ErrorMsg:  msg,
			ErrorPath: ErrorPath,
			Function:  Function,
		},
	}
}
