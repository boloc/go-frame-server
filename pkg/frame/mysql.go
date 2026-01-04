package frame

import (
	"github.com/boloc/go-frame-server/pkg/frame/components"

	"gorm.io/gorm"
)

// DefaultDB 获取默认实例的主库连接
func DefaultDB() *gorm.DB {
	return components.DefaultMasterDB()
}

// DefaultSlaveDB 获取默认实例的从库连接
func DefaultSlaveDB() *gorm.DB {
	return components.DefaultSlaveDB()
}

// MasterDB 获取指定实例的主库连接
func MasterDB(name string) *gorm.DB {
	return components.MasterDB(name)
}

// SlaveDB 获取指定实例的从库连接
func SlaveDB(name string) *gorm.DB {
	return components.SlaveDB(name)
}
