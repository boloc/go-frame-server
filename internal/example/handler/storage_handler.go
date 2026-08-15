package handler

import (
	"time"

	"github.com/boloc/go-frame-server/internal/example/dto"
	"github.com/boloc/go-frame-server/pkg/errs"
	"github.com/boloc/go-frame-server/pkg/frame/storage"
	"github.com/boloc/go-frame-server/pkg/frame/webx"
	"github.com/gin-gonic/gin"
)

// 这一组函数演示 pkg/frame/storage 的用法，路由挂在 /test/storage 下，不对应真实业务。
// 需要在 config/frame-server.yml 配置好 storage.r2 才能真正调通，见该包的 README。

// storageUnavailable 统一"对象存储未配置"时的错误文案。
func storageUnavailable() error {
	return errs.New(errs.CodeDependency, "对象存储未配置，请检查 storage.r2 配置项")
}

// StorageUpload 上传一个文件。
// POST /test/storage/upload  multipart/form-data: file
func StorageUpload(c *gin.Context) {
	client, ok := storage.TryDefault()
	if !ok {
		webx.Fail(c, storageUnavailable())
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		webx.Fail(c, errs.InvalidParams("缺少 file 表单字段: "+err.Error()))
		return
	}

	result, err := client.UploadMultipartFile(c.Request.Context(), file, "", "")
	if err != nil {
		webx.Fail(c, errs.Dependency(err))
		return
	}
	webx.Success(c, result)
}

// StorageList 按前缀列出对象键。
// GET /test/storage/objects?prefix=&max_keys=
func StorageList(c *gin.Context) {
	client, ok := storage.TryDefault()
	if !ok {
		webx.Fail(c, storageUnavailable())
		return
	}

	var req dto.StorageListReq
	if err := webx.BindQuery(c, &req); err != nil {
		webx.Fail(c, err)
		return
	}

	keys, err := client.ListObjects(c.Request.Context(), req.Prefix, req.MaxKeys)
	if err != nil {
		webx.Fail(c, errs.Dependency(err))
		return
	}
	webx.Success(c, gin.H{"keys": keys})
}

// StorageDelete 删除一个对象。
// DELETE /test/storage/objects?key=...
func StorageDelete(c *gin.Context) {
	client, ok := storage.TryDefault()
	if !ok {
		webx.Fail(c, storageUnavailable())
		return
	}

	var req dto.StorageDeleteReq
	if err := webx.BindQuery(c, &req); err != nil {
		webx.Fail(c, err)
		return
	}

	if err := client.DeleteObject(c.Request.Context(), req.Key); err != nil {
		webx.Fail(c, errs.Dependency(err))
		return
	}
	webx.Success(c, nil)
}

// StoragePresignedURL 生成一个限时访问的预签名 URL。
// GET /test/storage/presigned-url?key=&expires_seconds=
func StoragePresignedURL(c *gin.Context) {
	client, ok := storage.TryDefault()
	if !ok {
		webx.Fail(c, storageUnavailable())
		return
	}

	var req dto.StoragePresignedURLReq
	if err := webx.BindQuery(c, &req); err != nil {
		webx.Fail(c, err)
		return
	}

	expires := time.Duration(req.ExpiresSeconds) * time.Second
	if expires <= 0 {
		expires = 15 * time.Minute
	}

	url, err := client.GeneratePresignedURL(c.Request.Context(), req.Key, expires)
	if err != nil {
		webx.Fail(c, errs.Dependency(err))
		return
	}
	webx.Success(c, gin.H{"url": url, "expires_in": expires.String()})
}
