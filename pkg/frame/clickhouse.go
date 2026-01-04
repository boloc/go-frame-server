package frame

import (
	"github.com/boloc/go-frame-server/pkg/frame/components"
	"gorm.io/gorm"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// DefaultClickHouse 获取默认ClickHouse原生连接
func DefaultClickHouse() driver.Conn {
	return components.GetDefaultClickHouse()
}

// ClickHouse 获取指定名称的ClickHouse原生连接
func ClickHouse(name string) driver.Conn {
	return components.GetClickHouse(name)
}

// DefaultClickHouseDB 获取默认ClickHouse GORM DB
func DefaultClickHouseDB() *gorm.DB {
	return components.DefaultClickHouseDB()
}

// ClickHouseDB 获取指定名称的ClickHouse GORM DB
func ClickHouseDB(name string) *gorm.DB {
	return components.ClickHouseDB(name)
}
