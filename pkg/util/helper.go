package util

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/url"
	"reflect"
	"time"

	"github.com/gin-gonic/gin"
)

// 拼接mysqlDSN
func BuildMysqlDSN(dbMap map[string]any) string {
	loc := url.QueryEscape(fmt.Sprint(dbMap["loc"]))
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=True&loc=%s",
		dbMap["user"],
		dbMap["password"],
		dbMap["host"],
		dbMap["port"],
		dbMap["name"],
		dbMap["charset"],
		loc,
	)
}

// 拼接clickhouseDSN
func BuildClickhouseDSN(dbMap map[string]any) string {
	loc := url.QueryEscape(fmt.Sprint(dbMap["loc"]))
	return fmt.Sprintf("http://%s:%s@%s:%s/%s?loc=%s",
		dbMap["user"],
		dbMap["password"],
		dbMap["host"],
		dbMap["port"],
		dbMap["name"],
		loc,
	)
}

// 获取客户端IP
func GetClientIP(c *gin.Context) string {
	return c.ClientIP()
}

// 打印请求体
func PrintReqParams(c *gin.Context) {
	// 判断方法类型
	method := c.Request.Method
	if method == "GET" {
		// 打印GET请求参数
		query := c.Request.URL.Query()
		fmt.Printf("传入的GET请求参数: %v\n", query)
		//转成json,带格式的
		queryJson, _ := json.MarshalIndent(query, "", "  ")
		fmt.Printf("传入的GET请求参数转json: %v\n", string(queryJson))
	} else if method == "POST" {
		// 打印POST请求参数
		body, _ := c.GetRawData()
		// 转成json,带格式的
		// bodyJson, _ := json.MarshalIndent(body, "", "  ")
		fmt.Printf("传入的body请求体: %s\n", string(body))
		// 为了不阻碍后续处理中还需要用到原始请求体，将数据重新设置回去
		c.Request.Body = io.NopCloser(bytes.NewBuffer(body))
	}
	fmt.Println("------------分割线--------------------")
}

// TimeToTimestamp 将事件字符串转换为 UTC 时间戳
func TimeToTimestamp(fromDatetime string, fromTimezone string) (int64, error) {
	layout := "2006-01-02 15:04:05" // 根据需要调整时间格式

	// 加载指定时区
	location, err := time.LoadLocation(fromTimezone)
	if err != nil {
		return 0, err
	}

	// 解析时间字符串为指定时区的时间
	t, err := time.ParseInLocation(layout, fromDatetime, location)
	if err != nil {
		return 0, err
	}

	// 返回 UTC 时间戳
	return t.UTC().Unix(), nil
}

// 格式化打印
func PrintFormat(obj any) {
	// 获取结构体类型
	typ := reflect.TypeOf(obj)
	// 获取结构体值
	val := reflect.ValueOf(obj)
	// 格式化成json输出
	json, _ := json.MarshalIndent(obj, "", "  ")
	// 打印结构体
	fmt.Printf("类型: %v\n", typ)
	fmt.Printf("值: %v\n", val)
	fmt.Printf("格式化: %v\n", string(json))
}

// 判断字符串是否在数组中
func Contains(arr []string, str string) bool {
	for _, v := range arr {
		if v == str {
			return true
		}
	}
	return false
}

// 递归解析 map[string]any，并返回一个 map
// 参数：
// valueMap: 需要解析的 map
// prefix: 前缀
// result: 结果 map
func ParseMap(valueMap map[string]any, prefix string, result map[string]any) {
	for key, value := range valueMap {
		fullKey := fmt.Sprintf("%s.%s", prefix, key) // 生成完整的键名
		switch v := value.(type) {
		case map[string]any:
			// 如果值是 map[string]any，递归调用
			ParseMap(v, fullKey, result)
		default:
			// 存储键值对到结果 map
			result[fullKey] = v
		}
	}
}

// 随机TTL，需要传入秒，在秒的基础上随机增加一些
func RandomTTL(seconds int) time.Duration {
	randomSeconds := rand.Intn(30)
	return time.Duration(seconds+randomSeconds) * time.Second
}
