package dto

import (
	"encoding/json"

	"github.com/boloc/go-frame-server/internal/example/enum"
	"github.com/boloc/go-frame-server/internal/model"
	"github.com/boloc/go-frame-server/pkg/frame/pagination"
	"github.com/boloc/go-frame-server/pkg/util"
)

// ==================== 请求 ====================

// ProductListReq 产品列表请求。Keyword 用 struct tag 做格式校验。
type ProductListReq struct {
	pagination.PageRequest        // 嵌入分页参数
	Status                 *int   `form:"status"`                              // 状态：1上架 0下架（不传则查全部）
	Keyword                string `form:"keyword" validate:"omitempty,max=50"` // 关键词搜索
	ColumnID               *uint  `form:"column_id"`                           // 栏目ID
}

// ToSearchCondition 转换为搜索条件。
func (r *ProductListReq) ToSearchCondition() ProductSearchCondition {
	return ProductSearchCondition{
		Status:   r.Status,
		Keyword:  r.Keyword,
		ColumnID: r.ColumnID,
	}
}

// ProductSearchCondition 产品搜索条件，供 handler/logic/repository 共用。
type ProductSearchCondition struct {
	Status   *int
	Keyword  string
	ColumnID *uint
}

// ProductUpdateStatusURI /:id 路径参数。
type ProductUpdateStatusURI struct {
	ID uint `uri:"id" validate:"required"`
}

// ProductDetailURI /:id 路径参数。
type ProductDetailURI struct {
	ID uint `uri:"id" validate:"required"`
}

// ProductUpdateStatusReq 更新产品状态请求。
type ProductUpdateStatusReq struct {
	Status int `json:"status" validate:"oneof=0 1"` // 1=上架(model.ProductStatusListed) 0=下架(model.ProductStatusUnlisted)
}

// ==================== 响应 ====================

// ProductItem 产品列表项
type ProductItem struct {
	ID           uint     `json:"id"`
	Title        string   `json:"title"`
	ColumnID     uint     `json:"column_id"`
	Sort         int      `json:"sort"`
	Status       int      `json:"status"`
	StatusText   string   `json:"status_text"`
	IsSkipDetail int      `json:"is_skip_detail"`
	Count        int      `json:"count"`
	Cover        string   `json:"cover"`
	CustomName   string   `json:"custom_name"`
	Tags         []string `json:"tags"`
	JumpUrl      string   `json:"jump_url"`
	CreatedAt    string   `json:"created_at"`
}

// ProductDetail 产品详情响应。Notice 来自 config_db 的全局公告，不是商品表字段。
type ProductDetail struct {
	ProductItem
	Notice string `json:"notice"` // 详情页全局公告，运营在后台改配置即时生效，没配置时为空字符串
}

// FromModel 从模型转换
func (p *ProductItem) FromModel(m *model.Product) *ProductItem {
	var tags []string
	if m.Tags != nil {
		_ = json.Unmarshal(m.Tags, &tags)
	}

	return &ProductItem{
		ID:           m.ID,
		Title:        m.Title,
		ColumnID:     m.ColumnID,
		Sort:         m.Sort,
		Status:       m.Status,
		StatusText:   statusText(m.Status),
		IsSkipDetail: m.IsSkipDetail,
		Count:        m.Count,
		Cover:        m.Cover,
		CustomName:   m.CustomName,
		Tags:         tags,
		JumpUrl:      m.JumpUrl,
		CreatedAt:    util.FormatLocal(m.CreatedAt),
	}
}

// statusText 状态文本，数据源是 enum.ProductStatus。
func statusText(status int) string {
	if label, ok := enum.ProductStatus.Get(status); ok {
		return label
	}
	return "未知"
}
