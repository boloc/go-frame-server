package model

import "time"

// OperationLog 存在 log_db 里的操作审计日志。
type OperationLog struct {
	ID         uint      `gorm:"primary_key;type:bigint;comment:主键ID"`
	Action     string    `gorm:"not null;type:varchar(50);comment:操作动作，例如 product.update_status"`
	TargetType string    `gorm:"not null;type:varchar(50);comment:操作对象类型，例如 product"`
	TargetID   uint      `gorm:"not null;type:bigint;comment:操作对象ID"`
	Detail     string    `gorm:"type:varchar(500);comment:操作详情"`
	CreatedAt  time.Time `gorm:"not null"`
}

// TableName 显式指定表名。
func (OperationLog) TableName() string {
	return "operation_logs"
}
