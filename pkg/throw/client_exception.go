package throw

import (
	"go-frame-server/pkg/throw/enum"
	"go-frame-server/pkg/throw/handler"
)

type ClientError struct {
	*handler.ExceptionError
}

// 请求客户端异常
func ClientException(err error) error {
	if err == nil {
		return nil
	}

	// 获取错误路径和函数名
	ErrorPath, Function := handler.ErrorCaller()
	return &ClientError{
		ExceptionError: &handler.ExceptionError{
			Code:      enum.NETWORK_REQUEST_ERROR,
			ErrorMsg:  err.Error(),
			ErrorPath: ErrorPath,
			Function:  Function,
		},
	}
}
