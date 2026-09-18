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

// 表名由 GORM 默认命名策略生成（operation_logs），并自动带上该实例在配置里的 prefix
// （database.<name>.prefix）。不要在这里实现 TableName()：显式返回固定表名会绕过
// NamingStrategy，配置里的 prefix 就静默失效了。
