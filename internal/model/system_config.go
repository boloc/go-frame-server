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

// TableName 显式指定表名。
func (SystemConfig) TableName() string {
	return "system_configs"
}
