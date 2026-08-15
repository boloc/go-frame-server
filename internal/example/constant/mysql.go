// Package constant 存放本示例应用自己的常量，避免各处手写字符串。
package constant

// MySQL 命名实例名称，对应配置 database.<name>。
const (
	MySQLDefaultDB = "default_db" // 主业务库，isDefault=true
	MySQLConfigDB  = "config_db"  // 系统配置库（见 internal/example/repository/system_config_repository.go）
	MySQLLogDB     = "log_db"     // 审计日志库（见 internal/example/repository/operation_log_repository.go）
)
