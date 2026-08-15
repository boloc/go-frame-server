package route

import (
	examplemw "github.com/boloc/go-frame-server/internal/example/middleware"

	"github.com/boloc/go-frame-server/internal/example/handler"
	"github.com/gin-gonic/gin"
)

// registerStorageRoutes 演示 pkg/frame/storage 的用法，挂在 /test/storage 下——跟
// registerCapabilityRoutes 一样是能力演示、不是真实业务接口，同样要求 X-Demo-Token；
// 额外原因：这几个接口会真的对配置好的对象存储桶产生读写副作用（上传/删除对象），
// 不应该被随便访问到。没配置 storage.r2（见 pkg/frame/storage/README.md）时，
// 这几个接口会返回"对象存储未配置"的业务错误，不会 panic。
//
//	curl -H "X-Demo-Token: x" -F "file=@/path/to/local.jpg" localhost:10006/test/storage/upload
//	curl -H "X-Demo-Token: x" "localhost:10006/test/storage/objects?prefix=uploads/"
//	curl -H "X-Demo-Token: x" -X DELETE "localhost:10006/test/storage/objects?key=uploads/xxx.jpg"
//	curl -H "X-Demo-Token: x" "localhost:10006/test/storage/presigned-url?key=uploads/xxx.jpg"
func registerStorageRoutes(r *gin.Engine) {
	storageGroup := r.Group("/test/storage")
	storageGroup.Use(examplemw.RequireDemoToken())
	{
		storageGroup.POST("/upload", handler.StorageUpload)
		storageGroup.GET("/objects", handler.StorageList)
		storageGroup.DELETE("/objects", handler.StorageDelete)
		storageGroup.GET("/presigned-url", handler.StoragePresignedURL)
	}
}
