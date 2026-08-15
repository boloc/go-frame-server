package handler

import (
	"github.com/boloc/go-frame-server/internal/example/dto"
	"github.com/boloc/go-frame-server/internal/example/logic"
	"github.com/boloc/go-frame-server/internal/example/validation"
	"github.com/boloc/go-frame-server/pkg/frame/webx"
	"github.com/gin-gonic/gin"
)

// OrderCreate 演示完整下单分层：绑定 -> 校验 -> logic -> repository，失败统一走 webx.Fail。
//
//	POST /api/orders  { "product_id": 1, "quantity": 2 }   -> 下单成功
//	POST /api/orders  { "product_id": 1, "quantity": 0 }   -> 字段格式校验失败
//	POST /api/orders  { "product_id": 4, "quantity": 1 }   -> 业务规则校验失败（已下架）
//	POST /api/orders  { "product_id": 2, "quantity": 1 }   -> 库存不足
//	POST /api/orders  { "product_id": 3, "quantity": 1 }   -> 产品不存在
//	POST /api/orders  { "product_id": 999, "quantity": 1 } -> 模拟数据库故障
func OrderCreate(c *gin.Context) {
	var req dto.OrderCreateReq
	if err := webx.BindJSON(c, &req); err != nil { // 绑定 + 字段格式校验（struct tag）
		webx.Fail(c, err)
		return
	}

	if err := validation.ValidateOrderCreate(&req); err != nil { // 显式的业务规则校验层
		webx.Fail(c, err)
		return
	}

	order, err := logic.NewOrderLogic().Create(&req)
	if err != nil {
		webx.Fail(c, err)
		return
	}

	webx.Success(c, order)
}
