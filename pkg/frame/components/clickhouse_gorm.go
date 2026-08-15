package components

import (
	"context"
	"fmt"
	"sync"
	"time"

	chgo "github.com/ClickHouse/clickhouse-go/v2"
	gormclickhouse "gorm.io/driver/clickhouse"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

// ClickHouseGORMConfig ClickHouse GORM 配置。
// 连接参数与 ClickHouseConfig 同名字段语义一致，经 buildClickHouseOptions 转成 clickhouse-go Options。
// DSN 非空时优先使用，忽略结构化字段。
type ClickHouseGORMConfig struct {
	DSN         string // 非空时优先用 DSN，忽略下面的结构化字段
	Address     []string
	Database    string
	Username    string
	Password    string
	Protocol    string // "http" 或者其它（含空字符串）都按 native 协议处理
	DialTimeout time.Duration
	ReadTimeout time.Duration
	Compression *chgo.Compression

	MaxIdleConns    int             // 最大空闲连接数
	MaxOpenConns    int             // 最大打开连接数
	ConnMaxLifetime time.Duration   // 连接最大生命周期
	ConnMaxIdleTime time.Duration   // 空闲超过该时间后从池中剔除
	LogLevel        logger.LogLevel // 日志级别
	Prefix          string          // 表前缀

	// ConnectRetryAttempts 启动时连接失败的重试次数（含第一次尝试），默认 3。
	ConnectRetryAttempts int
	// ConnectRetryInterval 每次重试之间的等待时间，默认 2s。
	ConnectRetryInterval time.Duration
}

// applyDefaults 设置默认值
func (c *ClickHouseGORMConfig) applyDefaults() {
	if c.MaxIdleConns == 0 {
		c.MaxIdleConns = 5
	}
	if c.MaxOpenConns == 0 {
		c.MaxOpenConns = 10
	}
	if c.ConnMaxLifetime == 0 {
		c.ConnMaxLifetime = time.Hour
	}
	if c.ConnMaxIdleTime == 0 {
		c.ConnMaxIdleTime = 10 * time.Minute
	}
	if c.LogLevel == 0 {
		c.LogLevel = logger.Warn
	}
	if c.ConnectRetryAttempts == 0 {
		c.ConnectRetryAttempts = 3
	}
	if c.ConnectRetryInterval == 0 {
		c.ConnectRetryInterval = 2 * time.Second
	}
}

// ClickHouseGORMComponent ClickHouse GORM组件
type ClickHouseGORMComponent struct {
	db     *gorm.DB
	config *ClickHouseGORMConfig
	mu     sync.RWMutex
}

var (
	clickhouseGormInstances = make(map[string]*ClickHouseGORMComponent)
	defaultClickHouseGORM   *ClickHouseGORMComponent
	clickhouseGormMu        sync.RWMutex
)

// NewClickHouseGORMComponent 创建 ClickHouse GORM 组件。同一 name 只能注册一次，重复注册会 panic。
func NewClickHouseGORMComponent(name string, config *ClickHouseGORMConfig, isDefault bool) *ClickHouseGORMComponent {
	clickhouseGormMu.Lock()
	defer clickhouseGormMu.Unlock()

	if _, exists := clickhouseGormInstances[name]; exists {
		panic(fmt.Sprintf("ClickHouse GORM instance [%s] already registered: NewClickHouseGORMComponent 不能对同一个 name 调用两次", name))
	}

	config.applyDefaults()
	c := &ClickHouseGORMComponent{config: config}
	clickhouseGormInstances[name] = c
	if isDefault {
		defaultClickHouseGORM = c
	}
	return c
}

// Start 启动ClickHouse GORM组件
func (c *ClickHouseGORMComponent) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Start 幂等：已连接则跳过。
	if c.db != nil {
		return nil
	}

	db, err := c.connectWithRetry(ctx)
	if err != nil {
		return fmt.Errorf("failed to connect to ClickHouse: %w", err)
	}

	c.db = db
	return nil
}

// connectWithRetry 按 ConnectRetryAttempts/ConnectRetryInterval 重试连接。
func (c *ClickHouseGORMComponent) connectWithRetry(ctx context.Context) (*gorm.DB, error) {
	var db *gorm.DB
	err := retryConnect(ctx, c.config.ConnectRetryAttempts, c.config.ConnectRetryInterval, func() error {
		var connectErr error
		db, connectErr = c.connect(ctx)
		return connectErr
	})
	if err != nil {
		return nil, err
	}
	return db, nil
}

// connect 用 clickhouse-go Options + OpenDB 建连，再交给 gorm.io/driver/clickhouse。
func (c *ClickHouseGORMComponent) connect(ctx context.Context) (*gorm.DB, error) {
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
		return nil, fmt.Errorf("failed to parse DSN: %w", err)
	}

	sqlDB := chgo.OpenDB(options)
	sqlDB.SetConnMaxLifetime(c.config.ConnMaxLifetime)
	sqlDB.SetConnMaxIdleTime(c.config.ConnMaxIdleTime)
	sqlDB.SetMaxIdleConns(c.config.MaxIdleConns)
	sqlDB.SetMaxOpenConns(c.config.MaxOpenConns)

	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}

	gormConfig := &gorm.Config{
		Logger: logger.Default.LogMode(c.config.LogLevel),
	}
	if c.config.Prefix != "" {
		gormConfig.NamingStrategy = schema.NamingStrategy{
			TablePrefix: c.config.Prefix,
		}
	}

	db, err := gorm.Open(gormclickhouse.New(gormclickhouse.Config{Conn: sqlDB}), gormConfig)
	if err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return db, nil
}

// Stop 停止ClickHouse GORM组件
func (c *ClickHouseGORMComponent) Stop(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.db == nil {
		return nil
	}
	sqlDB, err := c.db.DB()
	if err != nil {
		return fmt.Errorf("failed to get sql.DB: %w", err)
	}
	if err := sqlDB.Close(); err != nil {
		return fmt.Errorf("failed to close ClickHouse connection: %w", err)
	}
	c.db = nil
	return nil
}

// DB 获取GORM DB实例
func (c *ClickHouseGORMComponent) DB() *gorm.DB {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.db
}

// ==================== 默认实例访问方法 ====================
//
// panic 版访问器适合启动阶段；运行时请用对应的 Try 版本。

// TryDefaultClickHouseDB 获取默认 ClickHouse GORM 实例的连接；未注册时返回 (nil, false)，不 panic。
func TryDefaultClickHouseDB() (*gorm.DB, bool) {
	clickhouseGormMu.RLock()
	instance := defaultClickHouseGORM
	clickhouseGormMu.RUnlock()

	if instance == nil {
		return nil, false
	}
	return instance.DB(), true
}

// DefaultClickHouseDB 获取默认 ClickHouse GORM DB；未注册时 panic。运行时请用 TryDefaultClickHouseDB。
func DefaultClickHouseDB() *gorm.DB {
	db, ok := TryDefaultClickHouseDB()
	if !ok {
		panic("default ClickHouse GORM instance not initialized")
	}
	return db
}

// ==================== 命名实例访问方法 ====================

// TryGetClickHouseGORMComponent 获取指定名称的ClickHouse GORM组件；不存在时返回 (nil, false)，不 panic。
func TryGetClickHouseGORMComponent(name string) (*ClickHouseGORMComponent, bool) {
	clickhouseGormMu.RLock()
	defer clickhouseGormMu.RUnlock()
	instance, ok := clickhouseGormInstances[name]
	return instance, ok
}

// TryClickHouseDB 获取指定名称ClickHouse GORM实例的连接；不存在时返回 (nil, false)，不 panic。
func TryClickHouseDB(name string) (*gorm.DB, bool) {
	instance, ok := TryGetClickHouseGORMComponent(name)
	if !ok {
		return nil, false
	}
	return instance.DB(), true
}

// ClickHouseDB 获取指定名称的 ClickHouse GORM DB；不存在时 panic。运行时请用 TryClickHouseDB。
func ClickHouseDB(name string) *gorm.DB {
	db, ok := TryClickHouseDB(name)
	if !ok {
		panic(fmt.Sprintf("ClickHouse GORM instance [%s] not found", name))
	}
	return db
}

// GetClickHouseGORMComponent 获取指定名称的 ClickHouse GORM 组件；不存在时 panic。运行时请用 TryGetClickHouseGORMComponent。
func GetClickHouseGORMComponent(name string) *ClickHouseGORMComponent {
	instance, ok := TryGetClickHouseGORMComponent(name)
	if !ok {
		panic(fmt.Sprintf("ClickHouse GORM instance [%s] not found", name))
	}
	return instance
}
