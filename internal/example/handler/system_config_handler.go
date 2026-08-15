package handler

import (
	"github.com/boloc/go-frame-server/internal/example/dto"
	"github.com/boloc/go-frame-server/internal/example/repository"
	"github.com/boloc/go-frame-server/pkg/errs"
	"github.com/boloc/go-frame-server/pkg/frame/webx"
	"github.com/gin-gonic/gin"
)

// SystemConfigGetByKey 按 key 查询一条系统配置（读 config_db 从库）。
// GET /api/system-configs/:key
func SystemConfigGetByKey(c *gin.Context) {
	key := c.Param("key")
	if key == "" {
		webx.Fail(c, errs.InvalidParams("key 不能为空"))
		return
	}

	value, err := repository.NewSystemConfigRepository().GetValue(c.Request.Context(), key)
	if err != nil {
		webx.Fail(c, err)
		return
	}
	webx.Success(c, gin.H{"key": key, "value": value})
}

// SystemConfigSet 写入一条系统配置（写 config_db 主库；存在则更新）。
// POST /api/system-configs/:key  { "value": "xxx" }
func SystemConfigSet(c *gin.Context) {
	key := c.Param("key")
	if key == "" {
		webx.Fail(c, errs.InvalidParams("key 不能为空"))
		return
	}

	var req dto.SystemConfigSetReq
	if err := webx.BindJSON(c, &req); err != nil {
		webx.Fail(c, err)
		return
	}

	if err := repository.NewSystemConfigRepository().SetValue(c.Request.Context(), key, req.Value); err != nil {
		webx.Fail(c, err)
		return
	}
	webx.Success(c, nil)
}
