package pagination

import "gorm.io/gorm"

// 默认分页配置
const (
	DefaultPage     = 1   // 默认页码
	DefaultPageSize = 20  // 默认每页条数
	MaxPageSize     = 500 // 最大每页条数
)

// ==================== 请求参数 ====================

//	通用分页请求参数
//
// 使用方式：嵌入到请求结构体中
//
//	type ListUserRequest struct {
//	    pagination.PageRequest
//	    Status int `form:"status"`
//	}
type PageRequest struct {
	Page     int `form:"page" json:"page"`           // 页码，从1开始
	PageSize int `form:"page_size" json:"page_size"` // 每页条数
}

//	规范化分页参数，返回处理后的 page 和 pageSize
//
// 不修改原始值，返回规范化后的值
func (r *PageRequest) Normalize() (page, pageSize int) {
	page = r.Page
	pageSize = r.PageSize

	if page <= 0 {
		page = DefaultPage
	}
	if pageSize <= 0 {
		pageSize = DefaultPageSize
	} else if pageSize > MaxPageSize {
		pageSize = MaxPageSize
	}

	return page, pageSize
}

// 计算偏移量
func (r *PageRequest) Offset() int {
	page, pageSize := r.Normalize()
	return (page - 1) * pageSize
}

// Limit 获取限制条数
func (r *PageRequest) Limit() int {
	_, pageSize := r.Normalize()
	return pageSize
}

// ==================== GORM Scope ====================

//	返回 GORM 分页 Scope，用于链式调用
//
// 使用方式：db.Scopes(req.Scope()).Find(&users)
func (r *PageRequest) Scope() func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Offset(r.Offset()).Limit(r.Limit())
	}
}

//	分页 Scope（独立函数版本）
//
// 使用方式：db.Scopes(pagination.Paginate(1, 20)).Find(&users)
func Paginate(page, pageSize int) func(db *gorm.DB) *gorm.DB {
	req := &PageRequest{Page: page, PageSize: pageSize}
	return req.Scope()
}

// ==================== 响应结构 ====================

// 通用分页响应
type PageResponse struct {
	List       any   `json:"list"`        // 数据列表
	Total      int64 `json:"total"`       // 总记录数
	Page       int   `json:"page"`        // 当前页码
	PageSize   int   `json:"page_size"`   // 每页条数
	TotalPages int   `json:"total_pages"` // 总页数
}

// 创建分页响应（内部使用）
func newPageResponse(list any, total int64, page, pageSize int) *PageResponse {
	totalPages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		totalPages++
	}

	return &PageResponse{
		List:       list,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}
}

//	从请求参数创建分页响应
//
// 使用方式：pagination.FromRequest(&req.PageRequest, users, total)
func FromRequest(req *PageRequest, list any, total int64) *PageResponse {
	page, pageSize := req.Normalize()
	return newPageResponse(list, total, page, pageSize)
}
