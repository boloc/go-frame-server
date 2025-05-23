package frame

import (
	"github.com/boloc/go-frame-server/pkg/frame/components"
	"gorm.io/gorm"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// 默认实例的全局访问方法
func DefaultClickHouse() driver.Conn {
	return components.GetDefaultClickHouse()
}

// 指定名称的ClickHouse实例访问方法
func ClickHouse(name string) driver.Conn {
	return components.GetClickHouse(name)
}

// DefaultClickHouseDB 获取默认ClickHouse GORM DB
func DefaultClickHouseDB() *gorm.DB {
	return components.GetDefaultClickHouseGORM()
}

// ClickHouseDB 获取指定名称的ClickHouse GORM DB
func ClickHouseDB(name string) *gorm.DB {
	return components.GetClickHouseGORMDB(name)
}
