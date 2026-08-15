package repository

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/boloc/go-frame-server/internal/example/dto"
	"github.com/boloc/go-frame-server/pkg/errs"
	"github.com/boloc/go-frame-server/pkg/logger"
	"go.uber.org/zap"
)

// OrderRepository 订单仓储。用内存 map 模拟持久化，方便 demo 不依赖 MySQL。
type OrderRepository struct {
	mu     sync.Mutex
	stock  map[uint]int // productID -> 剩余库存
	nextID uint
	orders map[uint]*dto.OrderResp
}

// productNotFoundStock 之外的产品 ID 都视为不存在；999 专门用来演示"数据库挂了"的错误路径。
const simulatedDBFailureProductID uint = 999

// NewOrderRepository 创建订单仓储，预置两个产品：1 号有货，2 号售罄。
func NewOrderRepository() *OrderRepository {
	return &OrderRepository{
		stock:  map[uint]int{1: 10, 2: 0},
		nextID: 1,
		orders: make(map[uint]*dto.OrderResp),
	}
}

// defaultOrderRepo 进程内单例：库存状态必须跨请求保持。
var defaultOrderRepo = NewOrderRepository()

// DefaultOrderRepository 返回进程内单例订单仓储。
func DefaultOrderRepository() *OrderRepository {
	return defaultOrderRepo
}

// FindStock 查询产品剩余库存。不存在返回 NotFound；999 模拟数据库故障。
func (r *OrderRepository) FindStock(productID uint) (int, error) {
	if productID == simulatedDBFailureProductID {
		return 0, errs.Database(errors.New("connection refused: dial tcp 127.0.0.1:3306"))
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	stock, ok := r.stock[productID]
	if !ok {
		return 0, errs.NotFound(fmt.Sprintf("产品 #%d 不存在", productID))
	}
	return stock, nil
}

// DeductAndCreateOrder 在同一把锁内扣库存并生成订单；库存不足返回 false。
func (r *OrderRepository) DeductAndCreateOrder(productID uint, quantity int) (*dto.OrderResp, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.stock[productID] < quantity {
		return nil, false
	}
	r.stock[productID] -= quantity

	order := &dto.OrderResp{
		ID:        r.nextID,
		ProductID: productID,
		Quantity:  quantity,
		Status:    "created",
	}
	r.orders[order.ID] = order
	r.nextID++
	return order, true
}

// lowStockThreshold 库存低于此值打警告日志。
const lowStockThreshold = 5

// ReportLowStock 检查库存，低于阈值打警告。签名与 cron.Task.Run 一致，可直接当任务函数。
func (r *OrderRepository) ReportLowStock(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for productID, stock := range r.stock {
		if stock < lowStockThreshold {
			logger.Warn("order: low stock detected",
				zap.Uint("product_id", productID), zap.Int("stock", stock))
		}
	}
	return nil
}
