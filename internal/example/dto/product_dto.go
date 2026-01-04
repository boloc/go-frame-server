package dto

import (
	"encoding/json"

	"github.com/boloc/go-frame-server/internal/example/repository"
	"github.com/boloc/go-frame-server/internal/model"
	"github.com/boloc/go-frame-server/pkg/frame/pagination"
)

// ==================== 请求 ====================

// ProductListReq 产品列表请求
type ProductListReq struct {
	pagination.PageRequest        // 嵌入分页参数
	Status                 *int   `form:"status"`    // 状态：1上架 0下架（不传则查全部）
	Keyword                string `form:"keyword"`   // 关键词搜索
	ColumnID               *uint  `form:"column_id"` // 栏目ID
}

// ToSearchCondition 转换为搜索条件
func (r *ProductListReq) ToSearchCondition() *repository.ProductSearchCondition {
	return &repository.ProductSearchCondition{
		Status:   r.Status,
		Keyword:  r.Keyword,
		ColumnID: r.ColumnID,
	}
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
		CreatedAt:    m.CreatedAt.Format("2006-01-02 15:04:05"),
	}
}

// statusText 状态文本
func statusText(status int) string {
	switch status {
	case 1:
		return "上架"
	case 0:
		return "下架"
	default:
		return "未知"
	}
}
