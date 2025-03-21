package enum

const (
	SUCCESS                         = 0    // 成功
	MOVED_PERMANENTLY               = 3010 // 永久重定向
	FOUNT                           = 3020 // 临时重定向
	SEE_OTHER                       = 3030 // 查看其他
	NOT_MODIFIED                    = 3040 // 未修改
	TEMPORARY_REDIRECT              = 3070 // 临时重定向
	BAD_REQUEST                     = 4000 // 错误请求
	BAD_REQUEST_VALIDATION          = 4001 // 参数验证错误
	UNAUTHORIZED                    = 4010 // 未授权
	TIMESTAMP_EXPIRED               = 4011 // 过期
	FORBIDDEN                       = 4030 // 禁止
	NOT_FOUND                       = 4040 // 未找到
	METHOD_NOT_ALLOWED              = 4050 // 方法不允许
	GONE                            = 4100 // 已删除
	UNSUPPORTED_MEDIA_TYPE          = 4150 // 不支持的媒体类型
	UNPROCESSABLE_ENTITY            = 4220 // 不可处理的实体
	TOO_MANY_REQUESTS               = 4290 // 太多请求
	SERVER_ERROR                    = 5000 // 服务器错误
	NOT_IMPLEMENTED                 = 5010 // 未实现
	BAD_GATEWAY                     = 5020 // 网关错误
	SERVICE_UNAVAILABLE             = 5030 // 服务不可用
	GATEWAY_TIMEOUT                 = 5040 // 网关超时
	HTTP_VERSION_NOT_SUPPORTED      = 5050 // HTTP版本不支持
	VARIANT_ALSO_NEGOTIATES         = 5060 // 变体也协商
	INSUFFICIENT_STORAGE            = 5070 // 存储不足
	LOOP_DETECTED                   = 5080 // 循环检测
	SQL_ERROR                       = 5090 // sql错误
	NOT_EXTENDED                    = 5100 // 未扩展
	NETWORK_AUTHENTICATION_REQUIRED = 5110 // 网络认证要求
	NETWORK_CONNECT_TIMEOUT_ERROR   = 5990 // 网络连接超时错误
	NETWORK_REQUEST_ERROR           = 5991 // 网络服务请求错误
)

type ApiCode struct {
	Code    int
	Message string
}

var apiCodes = []ApiCode{
	{Code: SUCCESS, Message: "success"},
	{Code: MOVED_PERMANENTLY, Message: "Moved Permanently"},
	{Code: FOUNT, Message: "Found"},
	{Code: SEE_OTHER, Message: "See Other"},
	{Code: NOT_MODIFIED, Message: "Not Modified"},
	{Code: TEMPORARY_REDIRECT, Message: "Temporary Redirect"},
	{Code: BAD_REQUEST, Message: "Bad Request"},
	{Code: UNAUTHORIZED, Message: "Unauthorized"},
	{Code: TIMESTAMP_EXPIRED, Message: "The request has expired"},
	{Code: FORBIDDEN, Message: "Forbidden"},
	{Code: NOT_FOUND, Message: "Not Found"},
	{Code: METHOD_NOT_ALLOWED, Message: "Method Not Allowed"},
	{Code: GONE, Message: "Gone"},
	{Code: UNSUPPORTED_MEDIA_TYPE, Message: "Unsupported Media Type"},
	{Code: UNPROCESSABLE_ENTITY, Message: "Unprocessable Entity"},
	{Code: TOO_MANY_REQUESTS, Message: "Too Many Requests"},
	{Code: SERVER_ERROR, Message: "Internal Server Error"},
	{Code: NOT_IMPLEMENTED, Message: "Not Implemented"},
	{Code: BAD_GATEWAY, Message: "Bad Gateway"},
	{Code: SERVICE_UNAVAILABLE, Message: "Service Unavailable"},
	{Code: GATEWAY_TIMEOUT, Message: "Gateway Timeout"},
	{Code: HTTP_VERSION_NOT_SUPPORTED, Message: "HTTP Version Not Supported"},
	{Code: VARIANT_ALSO_NEGOTIATES, Message: "Variant Also Negotiates"},
	{Code: INSUFFICIENT_STORAGE, Message: "Insufficient Storage"},
	{Code: LOOP_DETECTED, Message: "Loop Detected"},
	{Code: NOT_EXTENDED, Message: "Not Extended"},
	{Code: NETWORK_AUTHENTICATION_REQUIRED, Message: "Network Authentication Required"},
	{Code: NETWORK_CONNECT_TIMEOUT_ERROR, Message: "Network Connect Timeout Error"},
	{Code: NETWORK_REQUEST_ERROR, Message: "Network Request Error"},
	{Code: SQL_ERROR, Message: "Sql Error"},
}

// GetMessage returns the message for a given code
func GetMessage(code int) string {
	for _, apiCode := range apiCodes {
		if apiCode.Code == code {
			return apiCode.Message
		}
	}
	return "Error!"
}
