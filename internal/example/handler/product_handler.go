package handler

import (
	examplecache "github.com/boloc/go-frame-server/v2/internal/example/cache"
	"github.com/boloc/go-frame-server/v2/internal/example/dto"
	"github.com/boloc/go-frame-server/v2/internal/example/enum"
	"github.com/boloc/go-frame-server/v2/internal/example/logic"
	"github.com/boloc/go-frame-server/v2/pkg/errs"
	"github.com/boloc/go-frame-server/v2/pkg/frame/webx"
	"github.com/gin-gonic/gin"
)

// ProductList 获取产品列表（pagination.PageRequest + FromRequest）。
//
//	GET /api/products/list?page=1&page_size=20&status=1&keyword=xxx&column_id=1
//	curl "localhost:10006/api/products/list?page=1&page_size=20"
func ProductList(c *gin.Context) {
	var req dto.ProductListReq
	if err := webx.BindQuery(c, &req); err != nil {
		webx.Fail(c, err)
		return
	}

	pageData, err := logic.NewProductLogic().GetList(&req)
	if err != nil {
		webx.Fail(c, err)
		return
	}

	webx.Success(c, pageData)
}

// ProductActiveCountSummary 返回当前上架产品数量，读 refreshcache 内存值，不查库。
// Get 只在内存从未加载成功时同步回落 Loader；ok=false 表示 Loader 也失败了。
//
//	GET /api/products/summary
//	curl localhost:10006/api/products/summary
func ProductActiveCountSummary(c *gin.Context) {
	count, ok := examplecache.ProductActiveCount.Get(c.Request.Context())
	if !ok {
		// 缓存尚未就绪且同步回落也失败时用 New 构造错误，不要传 nil 给 Wrap。
		webx.Fail(c, errs.New(errs.CodeInternal, "产品数量统计暂不可用，请稍后重试"))
		return
	}
	webx.Success(c, gin.H{"active_count": count})
}

// ProductOptions 返回产品状态下拉选项，数据来自 enum.ProductStatus（pkg/util/options）。
//
//	GET /api/products/options
//	curl localhost:10006/api/products/options
func ProductOptions(c *gin.Context) {
	webx.Success(c, gin.H{
		"status_options": enum.ProductStatus.Options(),
	})
}

// ProductDetail 获取产品详情，并附带 config_db 里的全局公告（refreshcache.ProductNotice）。
//
//	GET /api/products/:id
//	curl localhost:10006/api/products/1
func ProductDetail(c *gin.Context) {
	var uri dto.ProductDetailURI
	if err := webx.BindURI(c, &uri); err != nil {
		webx.Fail(c, err)
		return
	}

	detail, err := logic.NewProductLogic().GetDetail(c.Request.Context(), uri.ID)
	if err != nil {
		webx.Fail(c, err)
		return
	}
	webx.Success(c, detail)
}

// ProductUpdateStatus 更新产品上架/下架状态。路径参数和 JSON body 分别绑定。
//
//	POST /api/products/:id/status  { "status": 0 }
//	curl -X POST -H "Content-Type: application/json" -d '{"status":0}' localhost:10006/api/products/1/status
func ProductUpdateStatus(c *gin.Context) {
	var uri dto.ProductUpdateStatusURI
	if err := webx.BindURI(c, &uri); err != nil {
		webx.Fail(c, err)
		return
	}

	var req dto.ProductUpdateStatusReq
	if err := webx.BindJSON(c, &req); err != nil {
		webx.Fail(c, err)
		return
	}

	if err := logic.NewProductLogic().UpdateStatus(c.Request.Context(), uri.ID, req.Status); err != nil {
		webx.Fail(c, err)
		return
	}
	webx.Success(c, nil)
}
