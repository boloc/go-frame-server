// Package bizerr 演示业务层如何在框架内置错误码之外声明并登记自己的错误码。
// 业务码从 10000 往上开，避免和框架内置码撞车。
package bizerr

import (
	"net/http"

	"github.com/boloc/go-frame-server/pkg/errs"
)

// 订单相关的业务错误码。
const (
	CodeOrderAlreadyPaid  errs.Code = 10001 // 订单已支付，不能重复支付
	CodeInsufficientStock errs.Code = 10002 // 库存不足，无法下单
)

// init 登记默认文案和 HTTP 状态码，包被导入时自动执行。
func init() {
	errs.RegisterHTTPStatus(CodeOrderAlreadyPaid, http.StatusConflict)
	errs.RegisterMessage(CodeOrderAlreadyPaid, "订单已支付，请勿重复支付")

	errs.RegisterHTTPStatus(CodeInsufficientStock, http.StatusConflict)
	errs.RegisterMessage(CodeInsufficientStock, "库存不足，下单失败")
}
