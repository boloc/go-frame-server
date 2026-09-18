package frame

import (
	"github.com/boloc/go-frame-server/v2/pkg/frame/components"
	"gorm.io/gorm"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// ClickHouse 是可选依赖（clickhouse.enabled=false 时根本不会注册），所以业务代码里
// 应该优先用 Try 版本，拿不到时返回明确的业务错误，而不是让 panic 版把请求打成 500。

// TryDefaultClickHouse 获取默认 ClickHouse 原生连接；未注册或未连接时返回 (nil, false)。
func TryDefaultClickHouse() (driver.Conn, bool) {
	return components.TryGetDefaultClickHouse()
}

// DefaultClickHouse 获取默认 ClickHouse 原生连接；未注册时 panic。可选依赖请用 TryDefaultClickHouse。
func DefaultClickHouse() driver.Conn {
	return components.GetDefaultClickHouse()
}

// TryClickHouse 获取指定名称的 ClickHouse 原生连接；不存在或未连接时返回 (nil, false)。
func TryClickHouse(name string) (driver.Conn, bool) {
	return components.TryGetClickHouse(name)
}

// ClickHouse 获取指定名称的 ClickHouse 原生连接；不存在时 panic。可选依赖请用 TryClickHouse。
func ClickHouse(name string) driver.Conn {
	return components.GetClickHouse(name)
}

// TryDefaultClickHouseDB 获取默认 ClickHouse GORM DB；未注册或未连接时返回 (nil, false)。
func TryDefaultClickHouseDB() (*gorm.DB, bool) {
	return components.TryDefaultClickHouseDB()
}

// DefaultClickHouseDB 获取默认 ClickHouse GORM DB；未注册时 panic。可选依赖请用 TryDefaultClickHouseDB。
func DefaultClickHouseDB() *gorm.DB {
	return components.DefaultClickHouseDB()
}

// TryClickHouseDB 获取指定名称的 ClickHouse GORM DB；不存在或未连接时返回 (nil, false)。
func TryClickHouseDB(name string) (*gorm.DB, bool) {
	return components.TryClickHouseDB(name)
}

// ClickHouseDB 获取指定名称的 ClickHouse GORM DB；不存在时 panic。可选依赖请用 TryClickHouseDB。
func ClickHouseDB(name string) *gorm.DB {
	return components.ClickHouseDB(name)
}
