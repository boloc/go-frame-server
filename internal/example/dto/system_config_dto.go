package dto

// SystemConfigSetReq 写入一条系统配置的请求体，配合 URL 上的 :key 路径参数使用。
type SystemConfigSetReq struct {
	Value string `json:"value" validate:"required,max=1000"`
}
