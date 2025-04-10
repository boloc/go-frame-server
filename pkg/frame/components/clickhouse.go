package components

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// ClickHouseConfig ClickHouse配置
type ClickHouseConfig struct {
	Address         []string                // 地址列表，支持集群
	Database        string                  // 数据库名
	Username        string                  // 用户名
	Password        string                  // 密码
	MaxOpenConns    int                     // 最大连接数
	MaxIdleConns    int                     // 最大空闲连接数
	ConnMaxLifetime time.Duration           // 连接最大生命周期
	DialTimeout     time.Duration           // 连接超时时间
	Compression     *clickhouse.Compression // 压缩方式
	Debug           bool                    // 调试
}

// ClickHouseComponent ClickHouse组件
type ClickHouseComponent struct {
	conn   driver.Conn // 连接
	config *ClickHouseConfig
	mu     sync.RWMutex
}

var (
	clickhouseInstances     = make(map[string]*ClickHouseComponent)
	clickhouseInstancesOnce = make(map[string]*sync.Once)
	DefaultClickHouse       *ClickHouseComponent // 添加默认实例
	clickhouseMu            sync.RWMutex
)

// ClickHouseOption 定义ClickHouse选项函数类型
type ClickHouseOption func(*ClickHouseConfig)

// WithClickHouseAddress 设置ClickHouse地址
func WithClickHouseAddress(address []string) ClickHouseOption {
	return func(c *ClickHouseConfig) {
		c.Address = address
	}
}

// WithClickHouseDebug 设置ClickHouse调试
func WithClickHouseDebug(debug bool) ClickHouseOption {
	return func(c *ClickHouseConfig) {
		c.Debug = debug
	}
}

// WithClickHouseDatabase 设置ClickHouse数据库
func WithClickHouseDatabase(database string) ClickHouseOption {
	return func(c *ClickHouseConfig) {
		c.Database = database
	}
}

// WithClickHouseUsername 设置ClickHouse用户名
func WithClickHouseUsername(username string) ClickHouseOption {
	return func(c *ClickHouseConfig) {
		c.Username = username
	}
}

// WithClickHousePassword 设置ClickHouse密码
func WithClickHousePassword(password string) ClickHouseOption {
	return func(c *ClickHouseConfig) {
		c.Password = password
	}
}

// WithClickHouseMaxOpenConns 设置最大连接数
func WithClickHouseMaxOpenConns(maxOpenConns int) ClickHouseOption {
	return func(c *ClickHouseConfig) {
		c.MaxOpenConns = maxOpenConns
	}
}

// WithClickHouseMaxIdleConns 设置最大空闲连接数
func WithClickHouseMaxIdleConns(maxIdleConns int) ClickHouseOption {
	return func(c *ClickHouseConfig) {
		c.MaxIdleConns = maxIdleConns
	}
}

// WithClickHouseConnMaxLifetime 设置连接最大生命周期
func WithClickHouseConnMaxLifetime(connMaxLifetime time.Duration) ClickHouseOption {
	return func(c *ClickHouseConfig) {
		c.ConnMaxLifetime = connMaxLifetime
	}
}

// WithClickHouseDialTimeout 设置连接超时时间
func WithClickHouseDialTimeout(dialTimeout time.Duration) ClickHouseOption {
	return func(c *ClickHouseConfig) {
		c.DialTimeout = dialTimeout
	}
}

// WithClickHouseCompression 设置压缩方式
func WithClickHouseCompression(method clickhouse.CompressionMethod) ClickHouseOption {
	return func(c *ClickHouseConfig) {
		c.Compression = &clickhouse.Compression{
			Method: method,
			Level:  0, // 使用默认压缩级别
		}
	}
}

// NewClickHouseComponent 创建ClickHouse组件
func NewClickHouseComponent(name string, isDefault bool, opts ...ClickHouseOption) *ClickHouseComponent {
	clickhouseMu.Lock()
	if _, exist := clickhouseInstancesOnce[name]; !exist {
		clickhouseInstancesOnce[name] = &sync.Once{}
	}
	once := clickhouseInstancesOnce[name]
	clickhouseMu.Unlock()

	once.Do(func() {
		config := &ClickHouseConfig{
			Address:         []string{"localhost:9000"},
			Database:        "default",
			Username:        "default",
			Password:        "",
			MaxOpenConns:    10,
			MaxIdleConns:    5,
			ConnMaxLifetime: time.Hour,
			DialTimeout:     10 * time.Second,
			Compression: &clickhouse.Compression{
				Method: clickhouse.CompressionLZ4,
				Level:  0, // 使用默认压缩级别
			},
			Debug: false,
		}

		for _, opt := range opts {
			opt(config)
		}

		c := &ClickHouseComponent{
			config: config,
		}

		clickhouseMu.Lock()
		clickhouseInstances[name] = c
		if isDefault {
			DefaultClickHouse = c
		}
		clickhouseMu.Unlock()
	})

	clickhouseMu.RLock()
	instance := clickhouseInstances[name]
	clickhouseMu.RUnlock()

	return instance
}

// Start 启动ClickHouse组件
func (c *ClickHouseComponent) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 如果已经连接，则跳过
	if c.conn != nil {
		return nil
	}

	options := &clickhouse.Options{
		Addr: c.config.Address, // 地址
		Auth: clickhouse.Auth{
			Database: c.config.Database, // 数据库
			Username: c.config.Username, // 用户名
			Password: c.config.Password, // 密码
		},
		DialTimeout:     c.config.DialTimeout,     // 连接超时时间
		MaxOpenConns:    c.config.MaxOpenConns,    // 最大连接数
		MaxIdleConns:    c.config.MaxIdleConns,    // 最大空闲连接数
		ConnMaxLifetime: c.config.ConnMaxLifetime, // 连接最大生命周期
		Compression:     c.config.Compression,     // 压缩方式
		Debug:           c.config.Debug,           // 调试
		Debugf: func(format string, v ...interface{}) { // 打印SQL(只有当Debug为true时，才会执行)
			msg := fmt.Sprintf(format, v...)
			if strings.Contains(msg, "send query") {
				// 使用ANSI颜色代码：绿色文本
				fmt.Printf("\033[32m执行的ClickHouse SQL: %s\033[0m\n", msg)
			}
		},
	}

	conn, err := clickhouse.Open(options)
	if err != nil {
		return fmt.Errorf("failed to connect to ClickHouse: %v", err)
	}

	// 测试连接
	if err := conn.Ping(ctx); err != nil {
		return fmt.Errorf("failed to ping ClickHouse: %v", err)
	}

	c.conn = conn
	return nil
}

// Stop 停止ClickHouse组件
func (c *ClickHouseComponent) Stop(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// GetConn 获取ClickHouse连接
func (c *ClickHouseComponent) GetConn() driver.Conn {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.conn
}

// 获取默认ClickHouse连接
func GetDefaultClickHouse() driver.Conn {
	if DefaultClickHouse == nil {
		panic("default ClickHouse instance not initialized")
	}
	return DefaultClickHouse.GetConn()
}

// 获取指定名称的ClickHouse连接
func GetClickHouse(name string) driver.Conn {
	clickhouseMu.RLock()
	instance, ok := clickhouseInstances[name]
	clickhouseMu.RUnlock()

	if !ok {
		panic(fmt.Sprintf("ClickHouse instance [%s] not found", name))
	}
	return instance.GetConn()
}

// 获取指定名称的ClickHouse组件
func GetClickHouseComponent(name string) *ClickHouseComponent {
	clickhouseMu.RLock()
	instance, ok := clickhouseInstances[name]
	clickhouseMu.RUnlock()

	if !ok {
		panic(fmt.Sprintf("ClickHouse instance [%s] not found", name))
	}
	return instance
}
