package dto

// StorageListReq 按前缀列出对象键。
type StorageListReq struct {
	Prefix  string `form:"prefix"`
	MaxKeys int32  `form:"max_keys"`
}

// StorageDeleteReq 删除一个对象。
type StorageDeleteReq struct {
	Key string `form:"key" validate:"required"`
}

// StoragePresignedURLReq 生成一个限时访问的预签名 URL。
type StoragePresignedURLReq struct {
	Key            string `form:"key" validate:"required"`
	ExpiresSeconds int    `form:"expires_seconds"` // 留空/<=0 时默认 15 分钟
}
