package model

import "time"

// ExampleClickHouseEvent 演示 ClickHouse GORM 读写的最小事件表。
// 表在首次调用 POST /test/clickhouse/events 时按需创建，不走 MySQL AutoMigrate。
type ExampleClickHouseEvent struct {
	EventID   string    `gorm:"column:event_id;primaryKey"`
	Name      string    `gorm:"column:name"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (ExampleClickHouseEvent) TableName() string { return "example_demo_events" }
