package repository

import (
	"github.com/boloc/go-frame-server/internal/model"
	"github.com/boloc/go-frame-server/pkg/frame"
	"github.com/boloc/go-frame-server/pkg/frame/pagination"
	"github.com/boloc/go-frame-server/pkg/throw"
	"gorm.io/gorm"
)

// ProductRepository 产品仓储层
type ProductRepository struct {
	db func() *gorm.DB
}

// NewProductRepository 创建产品仓储
func NewProductRepository() *ProductRepository {
	return &ProductRepository{
		db: frame.DefaultDB,
	}
}

// ProductSearchCondition 产品搜索条件
type ProductSearchCondition struct {
	Status   *int
	Keyword  string
	ColumnID *uint
}

// GetList 获取产品列表（分页）
func (r *ProductRepository) GetList(condition *ProductSearchCondition, pageReq *pagination.PageRequest) ([]*model.Product, int64, error) {
	var list []*model.Product
	var total int64

	query := r.db().Model(&model.Product{})
	query = r.applyConditions(query, condition)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, throw.SqlException(err)
	}

	if err := query.Scopes(pageReq.Scope()).
		Order("sort ASC, id DESC").
		Find(&list).Error; err != nil {
		return nil, 0, throw.SqlException(err)
	}

	return list, total, nil
}

// applyConditions 应用查询条件
func (r *ProductRepository) applyConditions(query *gorm.DB, condition *ProductSearchCondition) *gorm.DB {
	if condition == nil {
		return query
	}

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
