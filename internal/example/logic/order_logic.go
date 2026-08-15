package logic

import (
	"github.com/boloc/go-frame-server/internal/example/bizerr"
	"github.com/boloc/go-frame-server/internal/example/dto"
	"github.com/boloc/go-frame-server/internal/example/repository"
	"github.com/boloc/go-frame-server/pkg/errs"
)

// OrderLogic 订单业务逻辑：编排 repository、把"库存不足"这类业务规则翻译成明确的错误码。
type OrderLogic struct {
	orderRepo *repository.OrderRepository
}

// NewOrderLogic 创建订单业务逻辑。
func NewOrderLogic() *OrderLogic {
	return &OrderLogic{orderRepo: repository.DefaultOrderRepository()}
}

// Create 创建订单。产品不存在/数据库故障原样上抛；库存不足返回 CodeInsufficientStock。
func (l *OrderLogic) Create(req *dto.OrderCreateReq) (*dto.OrderResp, error) {
	stock, err := l.orderRepo.FindStock(req.ProductID)
	if err != nil {
		return nil, err
	}
	if stock < req.Quantity {
		return nil, errs.New(bizerr.CodeInsufficientStock, "")
	}

	order, ok := l.orderRepo.DeductAndCreateOrder(req.ProductID, req.Quantity)
	if !ok {
		// 极小概率的并发竞争：FindStock 和 DeductAndCreateOrder 之间库存被其它请求抢走了。
		return nil, errs.New(bizerr.CodeInsufficientStock, "库存刚被抢完，请重新下单")
	}
	return order, nil
}
