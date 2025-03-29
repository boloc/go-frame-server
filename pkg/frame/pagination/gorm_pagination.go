package pagination

import "gorm.io/gorm"

// PageRequest 通用分页请求参数
type PageRequest struct {
	Page     int `form:"page" json:"page"`           // 页码
	PageSize int `form:"page_size" json:"page_size"` // 每页条数
}

// GetPageInfo 获取分页信息，处理默认值(非必须)
// 如果需要处理默认值，可以调用此方法
// 默认每页20条，最大500条
func (r *PageRequest) GetPageInfo() (page, pageSize int) {
	// 默认第一页
	if r.Page <= 0 {
		r.Page = 1
	}

	// 默认每页20条，最大500条
	if r.PageSize <= 0 {
		r.PageSize = 20
	} else if r.PageSize > 500 {
		r.PageSize = 500
	}

	return r.Page, r.PageSize
}

// Paginate 分页
func Paginate(page, pageSize int) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if page <= 0 {
			page = 1
		}

		if pageSize <= 0 {
			pageSize = 20
		}

		offset := (page - 1) * pageSize
		return db.Offset(offset).Limit(pageSize)
	}
}

// PageResponse 通用分页响应
type PageResponse struct {
	Total      int64 `json:"total"`       // 总记录数
	Page       int   `json:"page"`        // 当前页码
	PageSize   int   `json:"page_size"`   // 每页条数
	TotalPages int   `json:"total_pages"` // 总页数
	List       any   `json:"list"`        // 数据列表
}

// NewPageResponse 创建分页响应
func NewPageResponse(list any, total int64, page, pageSize int) *PageResponse {
	totalPages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		totalPages++
	}

	return &PageResponse{
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
		List:       list,
	}
}
