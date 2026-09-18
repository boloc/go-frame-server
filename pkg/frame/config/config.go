package config

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/boloc/go-frame-server/pkg/constant"
	"github.com/boloc/go-frame-server/pkg/frame/validate"

	"github.com/go-viper/mapstructure/v2"
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

// GetConfig 返回 MustLoadFile 设置的全局单例。
// 不推荐：示例应用已改用 LoadFile 后显式传入 conf，见 LoadFile。
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
func (c *ConfigComponent) Get(key string) any {
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
func (c *ConfigComponent) Unmarshal(rawVal any) error {
	return c.viper.Unmarshal(rawVal)
}

// IsSet 判断 key 是否在配置中显式出现（含空值，如 trusted_proxies: []）。
func (c *ConfigComponent) IsSet(key string) bool {
	return c.viper.IsSet(key)
}

// StrictUnmarshalKey 把 key 对应的配置段解析到 out：未知字段报错，再跑 validate.Struct。
// 保留 viper 默认的 DecodeHook，否则 time.Duration / 逗号分隔切片会解不出来。
func (c *ConfigComponent) StrictUnmarshalKey(key string, out any) error {
	err := c.viper.UnmarshalKey(key, out, func(dc *mapstructure.DecoderConfig) {
		dc.ErrorUnused = true
		dc.DecodeHook = mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
			mapstructure.StringToSliceHookFunc(","),
			mapstructure.TextUnmarshallerHookFunc(),
		)
	})
	if err != nil {
		return fmt.Errorf("config: %s: %w", key, err)
	}
	if fieldErrs := validate.Struct(out); len(fieldErrs) > 0 {
		return formatValidateError(key, fieldErrs)
	}
	return nil
}

// MustStrictUnmarshalKey 同 StrictUnmarshalKey，失败则 panic（消息前缀 config: ）。
func (c *ConfigComponent) MustStrictUnmarshalKey(key string, out any) {
	if err := c.StrictUnmarshalKey(key, out); err != nil {
		panic(err)
	}
}

// formatValidateError 把 validate.Struct 的字段说明拼成：
// config: <key> 校验失败: master 不能为空; max_open_conns 不能小于 1
func formatValidateError(key string, fieldErrs map[string]string) error {
	names := make([]string, 0, len(fieldErrs))
	for name := range fieldErrs {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, fieldErrs[name])
	}
	return fmt.Errorf("config: %s 校验失败: %s", key, strings.Join(parts, "; "))
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

// ParseDuration 解析时间字符串。空字符串返回 (0, nil)；非空但非法返回 error。
func ParseDuration(value string) (time.Duration, error) {
	if value == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("解析时间配置 %q 失败: %w", value, err)
	}
	return d, nil
}

// Resolve 按 -c/--config、CONFIG_FILE、defaultPath 的优先级解析配置路径，不加载文件。
func Resolve(defaultPath string) string {
	if v := scanArgsForFlag(os.Args[1:], "c", "config"); v != "" {
		return v
	}
	if v := os.Getenv("CONFIG_FILE"); v != "" {
		return v
	}
	return defaultPath
}

// scanArgsForFlag 在 args 里查找 -name / --name（以及 name=value 形式），返回其值；找不到返回空串。
func scanArgsForFlag(args []string, names ...string) string {
	for i := range args {
		trimmed := strings.TrimLeft(args[i], "-")
		if trimmed == args[i] {
			continue
		}
		for _, name := range names {
			switch {
			case trimmed == name:
				if i+1 < len(args) {
					return args[i+1]
				}
			case strings.HasPrefix(trimmed, name+"="):
				return strings.TrimPrefix(trimmed, name+"=")
			}
		}
	}
	return ""
}

// LoadFile 加载配置文件，失败返回 error，不设置全局单例。
func LoadFile(configFile string) (*ConfigComponent, error) {
	conf := NewConfig(configFile)
	if err := conf.Load(); err != nil {
		return nil, err
	}
	return conf, nil
}

// MustLoadFile 加载配置并设为全局单例，失败则 panic。
// 不推荐：请用 LoadFile，由调用方持有 *ConfigComponent。
func MustLoadFile(configFile string) *ConfigComponent {
	once.Do(func() {
		conf, err := LoadFile(configFile)
		if err != nil {
			panic(err)
		}
		globalConfig = conf
	})
	return globalConfig
}

// IsProduction 判断全局单例的 server.env 是否为 production。
// 不推荐：请对 LoadFile 得到的 conf 读 server.env。
func IsProduction() bool {
	return GetConfig().GetString("server.env") == constant.EnvProd
}
