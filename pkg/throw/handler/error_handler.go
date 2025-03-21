package handler

import (
	"fmt"
	"runtime"
)

type ExceptionError struct {
	Code      int    `json:"code"`
	ErrorMsg  string `json:"message"`
	ErrorPath string `json:"error_path"`
	Function  string `json:"function"`
}

// 实现error接口
func (e *ExceptionError) Error() string {
	return e.ErrorMsg
}

// 记录错误调用者信息
func ErrorCaller() (string, string) {
	// 获取错误路径和函数名
	pc, file, line, ok := runtime.Caller(2)
	var ErrorPath string
	var Function string
	if ok {
		ErrorPath = fmt.Sprintf("%s:%d", file, line)
		Function = runtime.FuncForPC(pc).Name()
	}

	return ErrorPath, Function
}
