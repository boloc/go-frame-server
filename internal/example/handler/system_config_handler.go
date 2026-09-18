package handler

import (
	"github.com/boloc/go-frame-server/v2/internal/example/dto"
	"github.com/boloc/go-frame-server/v2/internal/example/repository"
	"github.com/boloc/go-frame-server/v2/pkg/frame/webx"
	"github.com/gin-gonic/gin"
)

// SystemConfigGetByKey 按 key 查询一条系统配置（读 config_db 从库）。
//
//	GET /api/system-configs/:key
//	curl localhost:10006/api/system-configs/product.notice
func SystemConfigGetByKey(c *gin.Context) {
	var uri dto.SystemConfigKeyURI
	if err := webx.BindURI(c, &uri); err != nil {
		webx.Fail(c, err)
		return
	}

	value, err := repository.NewSystemConfigRepository().GetValue(c.Request.Context(), uri.Key)
	if err != nil {
		webx.Fail(c, err)
		return
	}
	webx.Success(c, gin.H{"key": uri.Key, "value": value})
}

// SystemConfigSet 写入一条系统配置（写 config_db 主库；存在则更新）。
//
//	POST /api/system-configs/:key  { "value": "xxx" }
//	curl -X POST -H "Content-Type: application/json" -d '{"value":"hello"}' \
//	  localhost:10006/api/system-configs/product.notice
func SystemConfigSet(c *gin.Context) {
	var uri dto.SystemConfigKeyURI
	if err := webx.BindURI(c, &uri); err != nil {
		webx.Fail(c, err)
		return
	}

	var req dto.SystemConfigSetReq
	if err := webx.BindJSON(c, &req); err != nil {
		webx.Fail(c, err)
		return
	}

	if err := repository.NewSystemConfigRepository().SetValue(c.Request.Context(), uri.Key, req.Value); err != nil {
		webx.Fail(c, err)
		return
	}
	webx.Success(c, nil)
}
