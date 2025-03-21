package throw

import (
	"frame-server/pkg/throw/enum"
	"frame-server/pkg/throw/handler"

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

	if err, ok := err.(*mysql.MySQLError); ok {
		// 获取错误路径和函数名
		ErrorPath, Function := handler.ErrorCaller()
		sqlError := &SqlError{
			ExceptionError: &handler.ExceptionError{
				Code:      enum.SQL_ERROR,
				ErrorMsg:  err.Error(), // sql的错误信息
				ErrorPath: ErrorPath,
				Function:  Function,
			},
			Number: err.Number,
		}
		return sqlError
	}
	return err
}
