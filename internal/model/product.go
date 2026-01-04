package model

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type Product struct {
	ID           uint           `gorm:"primary_key;type:bigint;comment:主键ID"`
	Title        string         `gorm:"not null;type:varchar(40);comment:产品标题"`
	ColumnID     uint           `gorm:"not null;type:integer;comment:栏目ID(可以取到布局&样式)"`
	Sort         int            `gorm:"not null;default:1;type:smallint;comment:排序(越小越靠前)"`
	Status       int            `gorm:"not null;default:1;type:tinyint;comment:状态 1:正常 0:禁用"`
	IsSkipDetail int            `gorm:"not null;default:0;type:tinyint;comment:是否跳过详情页 1:跳过 0:不跳过"`
	Count        int            `gorm:"not null;default:0;type:integer;comment:产品浏览量/下载次数"`
	Cover        string         `gorm:"default:'';type:varchar(512);comment:产品封面"`
	Images       datatypes.JSON `gorm:"type:json;comment:详情图"`
	CustomName   string         `gorm:"default:'';type:varchar(80);comment:客户名称"`
	Tags         datatypes.JSON `gorm:"type:json;comment:自定义标签"`
	Content      string         `gorm:"type:text;comment:产品内容/描述"`
	JumpUrl      string         `gorm:"default:'';type:varchar(512);comment:产品跳转链接"`
	CreatedAt    time.Time      `gorm:"not null"`
	UpdatedAt    time.Time      `gorm:"not null"`
	DeletedAt    gorm.DeletedAt
}
