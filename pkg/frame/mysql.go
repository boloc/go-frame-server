package frame

import (
	"go-frame-server/pkg/frame/components"

	"gorm.io/gorm"
)

// 默认实例的全局访问方法
func DefaultDB() *gorm.DB {
	// 获取这个结构体内所有参数
	if components.DefaultDB == nil {
		panic("default MySQL instance not initialized")
	}
	return components.DefaultDB.Master()
}

// 默认实例的从库访问方法
func DefaultSlaveDB() *gorm.DB {
	if components.DefaultDB == nil {
		panic("default MySQL instance not initialized")
	}
	return components.DefaultDB.Slave()
}

// 保留原有的命名实例访问方法
func DB(name string) *gorm.DB {
	instance := components.GetMySQLComponent(name)
	return instance.Master()
}

// ReplicaDB 获取从库连接
func SlaveDB(name string) *gorm.DB {
	instance := components.GetMySQLComponent(name)
	return instance.Slave()
}
