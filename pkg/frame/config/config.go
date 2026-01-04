package config

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/boloc/go-frame-server/pkg/constant"

	"github.com/spf13/viper"
)

var (
	globalConfig *ConfigComponent
	once         sync.Once
)

// 配置组件
type ConfigComponent struct {
	viper      *viper.Viper
	configFile string // 配置文件完整路径
}

//	创建配置组件
//
// @param configFile string 配置文件完整路径，如: ./config/frame-server.yml
func NewConfig(configFile string) *ConfigComponent {
	return &ConfigComponent{
		viper:      viper.New(),
		configFile: configFile,
	}
}

// 获取全局配置实例
func GetConfig() *ConfigComponent {
	if globalConfig == nil {
		panic("global config not initialized")
	}
	return globalConfig
}

// 加载配置文件
func (c *ConfigComponent) Load() error {
	c.viper.SetConfigFile(c.configFile)

	if err := c.viper.ReadInConfig(); err != nil {
		return fmt.Errorf("failed to read config file: %v", err)
	}

	return nil
}

//	获取Viper实例
//
// @return *viper.Viper Viper实例
func (c *ConfigComponent) GetViper() *viper.Viper {
	return c.viper
}

//	获取配置
//
// @param key string 配置名
// @return interface{} 配置值
func (c *ConfigComponent) Get(key string) interface{} {
	return c.viper.Get(key)
}

//	获取字符串配置
//
// @param key string 配置名
// @return string 字符串
func (c *ConfigComponent) GetString(key string) string {
	return c.viper.GetString(key)
}

// 获取整数配置
func (c *ConfigComponent) GetInt(key string) int {
	return c.viper.GetInt(key)
}

// 获取int64配置
func (c *ConfigComponent) GetInt64(key string) int64 {
	return c.viper.GetInt64(key)
}

// GetInt32 获取int32配置
func (c *ConfigComponent) GetInt32(key string) int32 {
	return c.viper.GetInt32(key)
}

// 获取uint配置
func (c *ConfigComponent) GetUint(key string) uint {
	return c.viper.GetUint(key)
}

// 获取布尔配置
func (c *ConfigComponent) GetBool(key string) bool {
	return c.viper.GetBool(key)
}

//	将配置反序列化到结构体
//
// @param rawVal interface{} 结构体
// @return error 错误
func (c *ConfigComponent) Unmarshal(rawVal interface{}) error {
	return c.viper.Unmarshal(rawVal)
}

//	获取字符串映射
//
// @param key string 配置名
// @return map[string]any 字符串映射
func (c *ConfigComponent) GetStringMap(key string) map[string]any {
	return c.viper.GetStringMap(key)
}

//	获取字符串时间
//
// @param key string 配置名
// @return time.Duration 时间
func (c *ConfigComponent) GetStringTimeDuration(key string) time.Duration {
	return c.viper.GetDuration(key)
}

//	获取字符串切片
//
// @param key string 配置名
// @return []string 字符串切片
func (c *ConfigComponent) GetStringSlice(key string) []string {
	return c.viper.GetStringSlice(key)
}

//	将字符串值解析为 time.Duration
//
// 用于已从配置解析出的字符串值，如 "1h", "30m", "5s"
//
// @param value string 时间字符串
// @return time.Duration 时间
func ParseDuration(value string) time.Duration {
	if value == "" {
		return 0
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		return 0
	}
	return d
}

//	加载配置文件（单例），如果出错则panic
//
// @param configFile string 配置文件完整路径，如: ./config/frame-server.yml
// @return *ConfigComponent 配置组件
func MustLoadFile(configFile string) *ConfigComponent {
	once.Do(func() {
		conf := NewConfig(configFile)
		if err := conf.Load(); err != nil {
			panic(err)
		}
		globalConfig = conf
	})
	return globalConfig
}

// @title 获取配置值, 独立方法
// @description 获取配置值，如果配置不存在则返回默认值
// @param key string 配置名
// @param defaultValue interface{} any
// @return interface{} any
func GetConfigValue[T any](key string, defaultValue T) T {
	viper := GetConfig().GetViper() // 获取viper实例
	value := viper.Get(key)
	if value == nil {
		return defaultValue
	}

	switch any(defaultValue).(type) {
	case string:
		return any(viper.GetString(key)).(T)
	case bool:
		return any(viper.GetBool(key)).(T)
	case int:
		return any(viper.GetInt(key)).(T)
	case float64:
		return any(viper.GetFloat64(key)).(T)
	case []string:
		return any(viper.GetStringSlice(key)).(T)
	case []any: // 处理接口列表
		return any(viper.Get(key)).(T) // 直接返回获取的值
	case map[string]any:
		return any(viper.GetStringMap(key)).(T)
	case time.Duration:
		return any(viper.GetDuration(key)).(T)
	default:
		log.Printf("Type not supported: %T, returning default value\n", defaultValue)
		return defaultValue
	}
}

// IsProduction 判断是否是生产环境
func IsProduction() bool {
	return GetConfig().GetString("server.env") == constant.EnvProd
}
