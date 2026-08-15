// Package enum 存放本示例的业务枚举（合法取值与展示文案）。
package enum

import (
	"github.com/boloc/go-frame-server/internal/model"
	"github.com/boloc/go-frame-server/pkg/util/options"
)

// ProductStatus 产品状态取值与展示文案，Value 引用 model 层常量。
var ProductStatus = options.NewBuilder[int, string]().
	Put(model.ProductStatusListed, "上架").
	Put(model.ProductStatusUnlisted, "下架")
