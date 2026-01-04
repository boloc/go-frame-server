package logic

import (
	"github.com/boloc/go-frame-server/internal/example/dto"
	"github.com/boloc/go-frame-server/internal/example/repository"
	"github.com/boloc/go-frame-server/pkg/frame/pagination"
)

// ProductLogic 产品业务逻辑
type ProductLogic struct {
	productRepo *repository.ProductRepository
}

// NewProductLogic 创建产品业务逻辑
func NewProductLogic() *ProductLogic {
	return &ProductLogic{
		productRepo: repository.NewProductRepository(),
	}
}

// GetList 获取产品列表
func (l *ProductLogic) GetList(req *dto.ProductListReq) (*pagination.PageResponse, error) {
	// 转换为搜索条件
	condition := req.ToSearchCondition()

	// 调用 Repository 获取数据
	list, total, err := l.productRepo.GetList(condition, &req.PageRequest)
	if err != nil {
		return nil, err
	}

	// DTO 转换
	items := make([]*dto.ProductItem, len(list))
	for i, product := range list {
		items[i] = new(dto.ProductItem).FromModel(product)
	}

	// 构建分页响应
	return pagination.FromRequest(&req.PageRequest, items, total), nil
}
