package model

import "time"

// SystemConfig 存放在 config_db 里的系统配置项（key-value）。
type SystemConfig struct {
	ID          uint      `gorm:"primary_key;type:bigint;comment:主键ID"`
	Key         string    `gorm:"not null;uniqueIndex;type:varchar(100);comment:配置项唯一键"`
	Value       string    `gorm:"not null;type:varchar(1000);comment:配置值"`
	Description string    `gorm:"default:'';type:varchar(255);comment:配置说明"`
	UpdatedAt   time.Time `gorm:"not null"`
}

// 表名由 GORM 默认命名策略生成（system_configs），并自动带上该实例在配置里的 prefix
// （database.<name>.prefix）。不要在这里实现 TableName()：显式返回固定表名会绕过
// NamingStrategy，配置里的 prefix 就静默失效了。
