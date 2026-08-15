package dto

// ==================== 请求 ====================

// OrderCreateReq 创建订单请求。Quantity 的格式校验由 struct tag 完成；业务规则在 validation 包。
type OrderCreateReq struct {
	ProductID uint `json:"product_id" validate:"required"`
	Quantity  int  `json:"quantity" validate:"required,min=1,max=99"`
}

// ==================== 响应 ====================

// OrderResp 订单响应。
type OrderResp struct {
	ID        uint   `json:"id"`
	ProductID uint   `json:"product_id"`
	Quantity  int    `json:"quantity"`
	Status    string `json:"status"`
}
