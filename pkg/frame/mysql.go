package frame

import (
	"github.com/boloc/go-frame-server/pkg/frame/components"

	"gorm.io/gorm"
)

// DefaultDB 获取默认实例的主库连接；未注册时 panic。运行时请用 TryDefaultDB。
func DefaultDB() *gorm.DB {
	return components.DefaultMasterDB()
}

// TryDefaultDB 获取默认实例的主库连接；默认实例未注册时返回 (nil, false)，不 panic。
func TryDefaultDB() (*gorm.DB, bool) {
	return components.TryDefaultMasterDB()
}

// DefaultSlaveDB 获取默认实例的从库连接；未注册时 panic。运行时请用 TryDefaultSlaveDB。
func DefaultSlaveDB() *gorm.DB {
	return components.DefaultSlaveDB()
}

// TryDefaultSlaveDB 获取默认实例的从库连接；默认实例未注册时返回 (nil, false)，不 panic。
func TryDefaultSlaveDB() (*gorm.DB, bool) {
	return components.TryDefaultSlaveDB()
}

// MasterDB 获取指定实例的主库连接；实例不存在时 panic。运行时请用 TryMasterDB。
func MasterDB(name string) *gorm.DB {
	return components.MasterDB(name)
}

// TryMasterDB 获取指定实例的主库连接；实例不存在时返回 (nil, false)，不 panic。
func TryMasterDB(name string) (*gorm.DB, bool) {
	return components.TryMasterDB(name)
}

// SlaveDB 获取指定实例的从库连接；实例不存在时 panic。运行时请用 TrySlaveDB。
func SlaveDB(name string) *gorm.DB {
	return components.SlaveDB(name)
}

// TrySlaveDB 获取指定实例的从库连接；实例不存在时返回 (nil, false)，不 panic。
func TrySlaveDB(name string) (*gorm.DB, bool) {
	return components.TrySlaveDB(name)
}
