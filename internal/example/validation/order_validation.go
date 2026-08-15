// Package validation 做不查库的业务规则校验，与 dto 上的 struct tag 格式校验分开。
package validation

import (
	"github.com/boloc/go-frame-server/internal/example/dto"
	"github.com/boloc/go-frame-server/pkg/errs"
)

// bannedProductIDs 模拟运营临时下架、禁止购买的产品。
var bannedProductIDs = map[uint]bool{
	4: true,
}

// ValidateOrderCreate 校验创建订单请求的业务规则。
func ValidateOrderCreate(req *dto.OrderCreateReq) error {
	if bannedProductIDs[req.ProductID] {
		return errs.InvalidParams("该产品已下架，暂不支持购买")
	}
	return nil
}
