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
	ReadTimeout     time.Duration           // 读取超时时间
	Compression     *clickhouse.Compression // 压缩方式
	Debug           bool                    // 调试
	Protocol        string                  // 协议类型：native 或 http
	DSN             string                  // 直接设置DSN连接字符串

	// ConnectRetryAttempts 启动时连接失败的重试次数（含第一次尝试），默认 3。
	ConnectRetryAttempts int
	// ConnectRetryInterval 每次重试之间的等待时间，默认 2s。
	ConnectRetryInterval time.Duration
}

// ClickHouseComponent ClickHouse组件
type ClickHouseComponent struct {
	conn   driver.Conn // 连接
	config *ClickHouseConfig
	mu     sync.RWMutex
}

var (
	clickhouseInstances = make(map[string]*ClickHouseComponent)
	DefaultClickHouse   *ClickHouseComponent // 添加默认实例
	clickhouseMu        sync.RWMutex
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

// WithClickHouseReadTimeout 设置读取超时时间
func WithClickHouseReadTimeout(readTimeout time.Duration) ClickHouseOption {
	return func(c *ClickHouseConfig) {
		c.ReadTimeout = readTimeout
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

// WithClickHouseProtocol 设置ClickHouse协议
func WithClickHouseProtocol(protocol string) ClickHouseOption {
	return func(c *ClickHouseConfig) {
		c.Protocol = protocol
	}
}

// WithClickHouseDSN 直接设置DSN连接字符串
func WithClickHouseDSN(dsn string) ClickHouseOption {
	return func(c *ClickHouseConfig) {
		c.DSN = dsn
	}
}

// WithClickHouseConnectRetryAttempts 设置 Start() 阶段初次连接的重试次数（含第一次尝试），默认 3。
func WithClickHouseConnectRetryAttempts(attempts int) ClickHouseOption {
	return func(c *ClickHouseConfig) {
		c.ConnectRetryAttempts = attempts
	}
}

// WithClickHouseConnectRetryInterval 设置 Start() 阶段初次连接每次重试之间的等待时间，默认 2s。
func WithClickHouseConnectRetryInterval(interval time.Duration) ClickHouseOption {
	return func(c *ClickHouseConfig) {
		c.ConnectRetryInterval = interval
	}
}

// NewClickHouseComponent 创建 ClickHouse 组件。同一 name 只能注册一次，重复注册会 panic。
// Address 可传多个地址，负载均衡与故障切换由 clickhouse-go 处理。
func NewClickHouseComponent(name string, isDefault bool, opts ...ClickHouseOption) *ClickHouseComponent {
	clickhouseMu.Lock()
	defer clickhouseMu.Unlock()

	if _, exists := clickhouseInstances[name]; exists {
		panic(fmt.Sprintf("ClickHouse instance [%s] already registered: NewClickHouseComponent 不能对同一个 name 调用两次", name))
	}

	config := &ClickHouseConfig{
		Address:         []string{"localhost:9000"},
		Database:        "default",
		Username:        "default",
		Password:        "",
		MaxOpenConns:    10,
		MaxIdleConns:    5,
		ConnMaxLifetime: time.Hour,
		DialTimeout:     10 * time.Second,
		ReadTimeout:     20 * time.Second,
		Compression: &clickhouse.Compression{
			Method: clickhouse.CompressionLZ4,
			Level:  0, // 使用默认压缩级别
		},
		Debug:                false,
		Protocol:             "native", // 默认使用native协议
		ConnectRetryAttempts: 3,
		ConnectRetryInterval: 2 * time.Second,
	}

	for _, opt := range opts {
		opt(config)
	}

	c := &ClickHouseComponent{config: config}
	clickhouseInstances[name] = c
	if isDefault {
		DefaultClickHouse = c
	}
	return c
}

// Start 启动ClickHouse组件
func (c *ClickHouseComponent) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Start 幂等：已连接则跳过。
	if c.conn != nil {
		return nil
	}

	if c.config.Debug {
		if c.config.DSN != "" {
			fmt.Printf("[clickhouse] 使用DSN连接: %s\n", maskClickHouseDSN(c.config.DSN))
		} else {
			fmt.Printf("[clickhouse] 连接信息: address=%v database=%s username=%s protocol=%s dial_timeout=%s\n",
				c.config.Address, c.config.Database, c.config.Username, c.config.Protocol, c.config.DialTimeout)
		}
	}

	// DSN 优先，否则按结构化字段构建 Options（与 GORM 组件共用 buildClickHouseOptions）。
	options, err := buildClickHouseOptions(clickhouseConnParams{
		DSN:         c.config.DSN,
		Address:     c.config.Address,
		Database:    c.config.Database,
		Username:    c.config.Username,
		Password:    c.config.Password,
		Protocol:    c.config.Protocol,
		DialTimeout: c.config.DialTimeout,
		ReadTimeout: c.config.ReadTimeout,
		Compression: c.config.Compression,
	})
	if err != nil {
		return fmt.Errorf("failed to parse DSN: %w", err)
	}
	// 连接池参数在此覆盖，DSN 与结构化字段两条路径都会生效。
	options.MaxOpenConns = c.config.MaxOpenConns
	options.MaxIdleConns = c.config.MaxIdleConns
	options.ConnMaxLifetime = c.config.ConnMaxLifetime

	// 设置调试模式
	if c.config.Debug {
		options.Debug = true
		options.Debugf = func(format string, v ...interface{}) { // 打印SQL(只有当Debug为true时，才会执行)
			msg := fmt.Sprintf(format, v...)
			if strings.Contains(msg, "send query") {
				// 使用ANSI颜色代码：绿色文本
				fmt.Printf("\033[32m执行的ClickHouse SQL: %s\033[0m\n", msg)
			}
		}
	}

	// 打开连接并 Ping，失败按 ConnectRetryAttempts/ConnectRetryInterval 重试。
	var conn driver.Conn
	retryErr := retryConnect(ctx, c.config.ConnectRetryAttempts, c.config.ConnectRetryInterval, func() error {
		newConn, openErr := clickhouse.Open(options)
		if openErr != nil {
			return openErr
		}
		if pingErr := newConn.Ping(ctx); pingErr != nil {
			_ = newConn.Close()
			return pingErr
		}
		conn = newConn
		return nil
	})
	if retryErr != nil {
		return fmt.Errorf("failed to connect to ClickHouse: %w", retryErr)
	}

	c.conn = conn
	return nil
}

// maskClickHouseDSN 打日志前脱敏 DSN 里的密码（DSN 形如 http://user:pass@host:port/db）。
func maskClickHouseDSN(dsn string) string {
	at := strings.Index(dsn, "@")
	if at <= 0 {
		return dsn
	}
	colon := strings.LastIndex(dsn[:at], ":")
	if colon <= 0 {
		return dsn
	}
	return dsn[:colon+1] + "***" + dsn[at:]
}

// Stop 停止ClickHouse组件
func (c *ClickHouseComponent) Stop(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil // 清空引用，便于 Restart 后重新连接
	return err
}

// GetConn 获取ClickHouse连接
func (c *ClickHouseComponent) GetConn() driver.Conn {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.conn
}

// GetDefaultClickHouse 获取默认 ClickHouse 连接；未初始化时 panic。
func GetDefaultClickHouse() driver.Conn {
	if DefaultClickHouse == nil {
		panic("default ClickHouse instance not initialized")
	}
	return DefaultClickHouse.GetConn()
}

// GetClickHouse 获取指定名称的 ClickHouse 连接；不存在时 panic。
func GetClickHouse(name string) driver.Conn {
	clickhouseMu.RLock()
	instance, ok := clickhouseInstances[name]
	clickhouseMu.RUnlock()

	if !ok {
		panic(fmt.Sprintf("ClickHouse instance [%s] not found", name))
	}
	return instance.GetConn()
}

// GetClickHouseComponent 获取指定名称的 ClickHouse 组件；不存在时 panic。
func GetClickHouseComponent(name string) *ClickHouseComponent {
	clickhouseMu.RLock()
	instance, ok := clickhouseInstances[name]
	clickhouseMu.RUnlock()

	if !ok {
		panic(fmt.Sprintf("ClickHouse instance [%s] not found", name))
	}
	return instance
}
