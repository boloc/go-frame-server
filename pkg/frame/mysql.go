package frame

import (
	"github.com/boloc/go-frame-server/v2/pkg/frame/components"

	"gorm.io/gorm"
)

// 访问器约定（与 docs/api-conventions.md 一致）：
//   - 请求期的必需依赖（默认业务库）在 repository 里直接用 panic 版（DefaultDB/DefaultSlaveDB）：
//     Gin 服务期间它一定是连着的，真拿不到是部署事故，让 Gin 的 recover 打成 500 并告警比
//     每个仓储方法多一个 ok 分支更诚实；
//   - 健康检查、可选依赖（可能 enabled=false 的实例）、启动钩子里的探测用 Try 版。

// DefaultDB 获取默认实例的主库连接；未注册时 panic。健康检查/可选依赖请用 TryDefaultDB。
func DefaultDB() *gorm.DB {
	return components.DefaultMasterDB()
}

// TryDefaultDB 获取默认实例的主库连接；默认实例未注册时返回 (nil, false)，不 panic。
func TryDefaultDB() (*gorm.DB, bool) {
	return components.TryDefaultMasterDB()
}

// DefaultSlaveDB 获取默认实例的从库连接；未注册时 panic。健康检查/可选依赖请用 TryDefaultSlaveDB。
func DefaultSlaveDB() *gorm.DB {
	return components.DefaultSlaveDB()
}

// TryDefaultSlaveDB 获取默认实例的从库连接；默认实例未注册时返回 (nil, false)，不 panic。
func TryDefaultSlaveDB() (*gorm.DB, bool) {
	return components.TryDefaultSlaveDB()
}

// MasterDB 获取指定实例的主库连接；实例不存在时 panic。健康检查/可选依赖请用 TryMasterDB。
func MasterDB(name string) *gorm.DB {
	return components.MasterDB(name)
}

// TryMasterDB 获取指定实例的主库连接；实例不存在时返回 (nil, false)，不 panic。
func TryMasterDB(name string) (*gorm.DB, bool) {
	return components.TryMasterDB(name)
}

// SlaveDB 获取指定实例的从库连接；实例不存在时 panic。健康检查/可选依赖请用 TrySlaveDB。
func SlaveDB(name string) *gorm.DB {
	return components.SlaveDB(name)
}

// TrySlaveDB 获取指定实例的从库连接；实例不存在时返回 (nil, false)，不 panic。
func TrySlaveDB(name string) (*gorm.DB, bool) {
	return components.TrySlaveDB(name)
}
