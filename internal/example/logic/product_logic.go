package logic

import (
	"context"
	"fmt"

	examplecache "github.com/boloc/go-frame-server/internal/example/cache"
	"github.com/boloc/go-frame-server/internal/example/dto"
	"github.com/boloc/go-frame-server/internal/example/repository"
	"github.com/boloc/go-frame-server/pkg/alert"
	"github.com/boloc/go-frame-server/pkg/frame/pagination"
	"github.com/boloc/go-frame-server/pkg/logger"
	"go.uber.org/zap"
)

// ProductLogic 产品业务逻辑
type ProductLogic struct {
	productRepo      *repository.ProductRepository
	operationLogRepo *repository.OperationLogRepository
}

// NewProductLogic 创建产品业务逻辑。
func NewProductLogic() *ProductLogic {
	return &ProductLogic{
		productRepo:      repository.NewProductRepository(),
		operationLogRepo: repository.NewOperationLogRepository(),
	}
}

// GetList 获取产品列表。repository 返回的 *errs.Error 原样上抛。
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

// GetDetail 获取产品详情，并拼上详情页公告（读 examplecache.ProductNotice 缓存）。
func (l *ProductLogic) GetDetail(ctx context.Context, id uint) (*dto.ProductDetail, error) {
	product, err := l.productRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	detail := &dto.ProductDetail{ProductItem: *new(dto.ProductItem).FromModel(product)}

	// 拿不到公告（缓存未预热成功）就留空，不阻断详情页。
	if notice, ok := examplecache.ProductNotice.Get(ctx); ok {
		detail.Notice = notice
	} else {
		logger.Warn("product: 读取详情页公告缓存失败", zap.Uint("product_id", id))
	}

	return detail, nil
}

// UpdateStatus 更新产品状态，并 best-effort 写一条审计日志到 log_db。
func (l *ProductLogic) UpdateStatus(ctx context.Context, id uint, status int) error {
	if err := l.productRepo.UpdateStatus(ctx, id, status); err != nil {
		return err
	}

	// 审计日志失败不影响接口结果，只记录并告警。
	detail := fmt.Sprintf("status=%d", status)
	if err := l.operationLogRepo.Create(ctx, "product.update_status", "product", id, detail); err != nil {
		logger.Warn("product: 写审计日志失败", zap.Error(err), zap.Uint("product_id", id))
		alert.Notify(ctx, alert.Event{
			Scope:   "product",
			Name:    "update_status.audit_log",
			Message: "审计日志写入失败",
			Err:     err,
			Fields:  map[string]any{"product_id": id, "status": status},
		})
	}
	return nil
}
