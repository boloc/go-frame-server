package model

import (
	"time"

	"gorm.io/gorm"
)

type ShortLinkRelationship struct {
	ID           uint   `gorm:"primarykey"`
	ShortUrlCode string `gorm:"column:short_url_code;type:varchar(150);not null;uniqueIndex;comment:短链code"` // 增加唯一索引
	OriginalUrl  string `gorm:"column:original_url;type:varchar(2048);not null;comment:对应原始链接"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    gorm.DeletedAt
}
