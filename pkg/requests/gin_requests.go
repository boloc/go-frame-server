package requests

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
)

type Requests struct {
	RequestIP     string          `json:"request_ip"`
	RequestMethod string          `json:"request_method"`
	RequestPath   string          `json:"request_path"`
	RequestQuery  *datatypes.JSON `json:"request_query"`
	RequestBody   *datatypes.JSON `json:"request_body"`
	RequestHeader *http.Header    `json:"request_header"`
}

func (r *Requests) GetMethod() string {
	return r.RequestMethod
}

func (r *Requests) GetPath() string {
	return r.RequestPath
}

func (r *Requests) GetBody() string {
	return r.RequestBody.String()
}

func (r *Requests) GetQuery() string {
	return r.RequestQuery.String()
}

func NewRequests(c *gin.Context) *Requests {
	// 判断方法类型
	var (
		requestQuery datatypes.JSON
		requestBody  datatypes.JSON
	)

	// 打印GET请求参数
	query := c.Request.URL.Query()
	requestQuery, _ = json.Marshal(query)
	if string(requestQuery) == "{}" { // 如果请求参数为空，则设置为nil
		requestQuery = nil
	}

	// 打印POST请求参数
	body, _ := c.GetRawData()
	requestBody = datatypes.JSON(body)
	if len(body) == 0 || string(requestBody) == "{}" { // 如果请求参数为空，则设置为nil
		requestBody = nil
	}

	// 重置请求体 PS:为了不阻碍后续处理中还需要用到原始请求体，将数据重新设置回去
	c.Request.Body = io.NopCloser(bytes.NewBuffer(body))

	return &Requests{
		RequestIP:     c.ClientIP(),
		RequestMethod: c.Request.Method,
		RequestPath:   c.Request.URL.Path,
		RequestHeader: &c.Request.Header,
		RequestQuery:  &requestQuery,
		RequestBody:   &requestBody,
	}
}
