package throw

import (
	"github.com/boloc/go-frame-server/pkg/throw/enum"
	"github.com/boloc/go-frame-server/pkg/throw/handler"

	"github.com/go-sql-driver/mysql"
)

type SqlError struct {
	*handler.ExceptionError
	Number uint16
}

// 数据库异常
func SqlException(err error) error {
	if err == nil {
		return nil
	}

	// 获取错误路径和函数名
	ErrorPath, Function := handler.ErrorCaller()

	// 初始化错误信息
	sqlError := &SqlError{
		ExceptionError: &handler.ExceptionError{
			Code:      enum.SQL_ERROR,
			ErrorMsg:  err.Error(), // sql的错误信息
			ErrorPath: ErrorPath,
			Function:  Function,
		},
	}

	// 获取Sql错误码
	if err, ok := err.(*mysql.MySQLError); ok {
		sqlError.Number = err.Number
		return sqlError
	}
	return sqlError
}
