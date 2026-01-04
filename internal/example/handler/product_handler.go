package handler

import (
	"github.com/boloc/go-frame-server/internal/example/dto"
	"github.com/boloc/go-frame-server/internal/example/logic"
	"github.com/boloc/go-frame-server/pkg/response"
	"github.com/gin-gonic/gin"
)

// ProductHandler 产品处理器
type ProductHandler struct {
	productLogic *logic.ProductLogic
}

// NewProductHandler 创建产品处理器
func NewProductHandler() *ProductHandler {
	return &ProductHandler{
		productLogic: logic.NewProductLogic(),
	}
}

//	获取产品列表
//
// GET /api/products?page=1&page_size=20&status=1&keyword=xxx&column_id=1
func (h *ProductHandler) List(c *gin.Context) {
	var req dto.ProductListReq
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequestError(c, err.Error())
		return
	}

	pageData, err := h.productLogic.GetList(&req)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, pageData)
}
