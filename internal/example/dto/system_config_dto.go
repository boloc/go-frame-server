package dto

// SystemConfigKeyURI /:key 路径参数。
type SystemConfigKeyURI struct {
	Key string `uri:"key" validate:"required"`
}

// SystemConfigSetReq 写入一条系统配置的请求体。
type SystemConfigSetReq struct {
	Value string `json:"value" validate:"required,max=1000"`
}
