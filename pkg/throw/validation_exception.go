package throw

import (
	"github.com/boloc/go-frame-server/pkg/throw/enum"
	"github.com/boloc/go-frame-server/pkg/throw/handler"

	"github.com/go-playground/validator/v10"
)

type ValidationError struct {
	*handler.ExceptionError
	Field            string
	Tag              string
	ValidationErrors []ValidationErrors
}

type ValidationErrors struct {
	Field string
	Tag   string
}

// func (e *ValidationError) Error() string {
// 	// 循环ValidationErrors获取所有的错误字段和tag
// 	errmsg := ""
// 	for _, validationError := range e.ValidationErrors {
// 		msg := fmt.Sprintf("Field [%s] failed on the [%s] ", validationError.Field, validationError.Tag)
// 		errmsg += msg + " &"
// 	}

// 	// 消除最后一个 &
// 	errmsg = strings.TrimRight(errmsg, "&")

// 	return errmsg
// }

// 参数验证异常
func ValidationException(err error, msg ...string) error {
	if err == nil {
		return nil
	}

	// 获取错误路径和函数名
	ErrorPath, Function := handler.ErrorCaller()

	// 自定义错误内容
	errMsg := err.Error()
	if len(msg) > 0 {
		errMsg = msg[0]
	}
	validationError := &ValidationError{
		ExceptionError: &handler.ExceptionError{
			Code:      enum.BAD_REQUEST,
			ErrorMsg:  errMsg,
			ErrorPath: ErrorPath,
			Function:  Function,
		},
	}

	if validationErrors, ok := err.(validator.ValidationErrors); ok {
		for _, fieldError := range validationErrors {
			validationError.Field = fieldError.Field()
			validationError.Tag = fieldError.Tag()
			validationError.ValidationErrors = append(validationError.ValidationErrors, ValidationErrors{
				Field: fieldError.Field(),
				Tag:   fieldError.Tag(),
			})
		}
		return validationError
	}
	return validationError
}
