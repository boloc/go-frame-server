package repository

import (
	"context"
	"errors"

	"github.com/boloc/go-frame-server/internal/example/dto"
	"github.com/boloc/go-frame-server/internal/model"
	"github.com/boloc/go-frame-server/pkg/errs"
	"github.com/boloc/go-frame-server/pkg/frame"
	"github.com/boloc/go-frame-server/pkg/frame/pagination"
	"gorm.io/gorm"
)

// ProductRepository 产品仓储。只读方法用 slave，写方法用 master。
type ProductRepository struct {
	master func() *gorm.DB
	slave  func() *gorm.DB
}

// NewProductRepository 创建产品仓储。master/slave 用方法值延迟取连接。
func NewProductRepository() *ProductRepository {
	return &ProductRepository{
		master: frame.DefaultDB,
		slave:  frame.DefaultSlaveDB,
	}
}

// GetList 获取产品列表（分页），读从库。
func (r *ProductRepository) GetList(condition dto.ProductSearchCondition, pageReq *pagination.PageRequest) ([]*model.Product, int64, error) {
	var list []*model.Product
	var total int64

	query := r.slave().Model(&model.Product{})
	query = r.applyConditions(query, condition)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, errs.Database(err)
	}

	if err := query.Scopes(pageReq.Scope()).
		Order("sort ASC, id DESC").
		Find(&list).Error; err != nil {
		return nil, 0, errs.Database(err)
	}

	return list, total, nil
}

// GetByID 按 ID 查询单个产品，读从库。不存在返回 NotFound，其它错误用 Database 包装。
func (r *ProductRepository) GetByID(ctx context.Context, id uint) (*model.Product, error) {
	var product model.Product
	err := r.slave().WithContext(ctx).Where("id = ?", id).First(&product).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.NotFound("产品不存在")
	}
	if err != nil {
		return nil, errs.Database(err)
	}
	return &product, nil
}

// CountActive 统计当前上架产品数量，读从库；供 refreshcache 演示作数据源。
func (r *ProductRepository) CountActive(ctx context.Context) (int64, error) {
	var count int64
	if err := r.slave().WithContext(ctx).Model(&model.Product{}).
		Where("status = ?", model.ProductStatusListed).Count(&count).Error; err != nil {
		return 0, errs.Database(err)
	}
	return count, nil
}

// UpdateStatus 更新产品上架/下架状态，写主库。
//
// 检查 RowsAffected：id 不存在（或已被软删）时 Update 只是更新 0 行，不会返回 error，
// 不检查的话会误报成功。注意：这个判断依赖 model.Product 有 UpdatedAt 字段（GORM 会
// 自动把 updated_at 带进 SET 子句，保证命中的行必然 RowsAffected >= 1）——如果以后
// 这里的 Update 改成显式禁用自动时间戳（Omit("updated_at") 之类），要重新考虑这个判断。
func (r *ProductRepository) UpdateStatus(ctx context.Context, id uint, status int) error {
	result := r.master().WithContext(ctx).Model(&model.Product{}).
		Where("id = ?", id).Update("status", status)
	if result.Error != nil {
		return errs.Database(result.Error)
	}
	if result.RowsAffected == 0 {
		return errs.NotFound("产品不存在")
	}
	return nil
}

// applyConditions 应用查询条件。
func (r *ProductRepository) applyConditions(query *gorm.DB, condition dto.ProductSearchCondition) *gorm.DB {
	if condition.Status != nil {
		query = query.Where("status = ?", *condition.Status)
	}
	if condition.Keyword != "" {
		query = query.Where("title LIKE ?", "%"+condition.Keyword+"%")
	}
	if condition.ColumnID != nil {
		query = query.Where("column_id = ?", *condition.ColumnID)
	}
	return query
}
