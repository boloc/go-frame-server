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

// ConfigComponent 配置组件
type ConfigComponent struct {
	viper    *viper.Viper
	confName string
	confPath string
}

// NewConfig 创建配置组件
func NewConfig(confName, confPath string) *ConfigComponent {
	return &ConfigComponent{
		viper:    viper.New(),
		confName: confName,
		confPath: confPath,
	}
}

// SetGlobalConfig 设置全局配置实例
func SetGlobalConfig(c *ConfigComponent) {
	once.Do(func() {
		globalConfig = c
	})
}

// GetGlobalConfig 获取全局配置实例
func GetConfig() *ConfigComponent {
	if globalConfig == nil {
		panic("global config not initialized")
	}
	return globalConfig
}

// Start 启动配置组件
func (c *ConfigComponent) Load() error {
	v := c.viper

	// 设置配置文件名
	v.SetConfigName(c.confName)

	// 设置配置文件路径
	v.AddConfigPath(c.confPath)
	// 设置配置文件类型
	v.SetConfigType("yaml")

	// 读取配置文件
	if err := v.ReadInConfig(); err != nil {
		return fmt.Errorf("failed to read config file: %v", err)
	}

	// // 支持环境变量覆盖
	// v.AutomaticEnv()
	// v.SetEnvPrefix(c.appName)

	return nil
}

// GetViper 获取Viper实例
func (c *ConfigComponent) GetViper() *viper.Viper {
	return c.viper
}

// Get 获取配置
func (c *ConfigComponent) Get(key string) interface{} {
	return c.viper.Get(key)
}

// GetString 获取字符串配置
func (c *ConfigComponent) GetString(key string) string {
	return c.viper.GetString(key)
}

// GetInt 获取整数配置
func (c *ConfigComponent) GetInt(key string) int {
	return c.viper.GetInt(key)
}

// GetBool 获取布尔配置
func (c *ConfigComponent) GetBool(key string) bool {
	return c.viper.GetBool(key)
}

// Unmarshal 将配置反序列化到结构体
func (c *ConfigComponent) Unmarshal(rawVal interface{}) error {
	return c.viper.Unmarshal(rawVal)
}

func (c *ConfigComponent) GetStringMap(key string) map[string]any {
	return c.viper.GetStringMap(key)
}

func (c *ConfigComponent) GetSrtingTimeDuration(key string) time.Duration {
	return c.viper.GetDuration(key)
}

func (c *ConfigComponent) GetStringSlice(key string) []string {
	return c.viper.GetStringSlice(key)
}

// MustLoad 创建并加载配置，如果出错则panic
func MustLoad(confName, confPath string) *ConfigComponent {
	conf := NewConfig(confName, confPath)
	if err := conf.Load(); err != nil {
		panic(err)
	}
	SetGlobalConfig(conf)
	return conf
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
