package components

import (
	"context"
	"fmt"
	"sync"
	"time"

	"gorm.io/driver/clickhouse"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

// ClickHouseGORMConfig ClickHouse GORM配置
type ClickHouseGORMConfig struct {
	DSN             string          // DSN连接字符串
	MaxIdleConns    int             // 最大空闲连接数
	MaxOpenConns    int             // 最大打开连接数
	ConnMaxLifetime time.Duration   // 连接最大生命周期
	LogLevel        logger.LogLevel // 日志级别
	Prefix          string          // 表前缀
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
	if c.LogLevel == 0 {
		c.LogLevel = logger.Warn
	}
}

// ClickHouseGORMComponent ClickHouse GORM组件
type ClickHouseGORMComponent struct {
	db     *gorm.DB
	config *ClickHouseGORMConfig
	mu     sync.RWMutex
}

var (
	clickhouseGormInstances     = make(map[string]*ClickHouseGORMComponent)
	clickhouseGormInstancesOnce = make(map[string]*sync.Once)
	defaultClickHouseGORM       *ClickHouseGORMComponent
	clickhouseGormMu            sync.RWMutex
)

// NewClickHouseGORMComponent 创建ClickHouse GORM组件
func NewClickHouseGORMComponent(name string, config *ClickHouseGORMConfig, isDefault bool) *ClickHouseGORMComponent {
	clickhouseGormMu.Lock()
	if _, exist := clickhouseGormInstancesOnce[name]; !exist {
		clickhouseGormInstancesOnce[name] = &sync.Once{}
	}
	once := clickhouseGormInstancesOnce[name]
	clickhouseGormMu.Unlock()

	once.Do(func() {
		// 兜底设置默认值
		config.applyDefaults()

		// 创建组件
		c := &ClickHouseGORMComponent{
			config: config,
		}

		clickhouseGormMu.Lock()
		clickhouseGormInstances[name] = c
		if isDefault {
			defaultClickHouseGORM = c
		}
		clickhouseGormMu.Unlock()
	})

	clickhouseGormMu.RLock()
	instance := clickhouseGormInstances[name]
	clickhouseGormMu.RUnlock()

	return instance
}

// Start 启动ClickHouse GORM组件
func (c *ClickHouseGORMComponent) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 如果已经连接，则跳过
	if c.db != nil {
		return nil
	}

	// 创建GORM配置
	gormConfig := &gorm.Config{
		Logger: logger.Default.LogMode(c.config.LogLevel),
	}

	// 判断是否需要前缀
	if c.config.Prefix != "" {
		gormConfig.NamingStrategy = schema.NamingStrategy{
			TablePrefix: c.config.Prefix,
		}
	}

	// 打开连接
	db, err := gorm.Open(clickhouse.Open(c.config.DSN), gormConfig)
	if err != nil {
		return fmt.Errorf("failed to connect to ClickHouse: %v", err)
	}

	// 设置连接池
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get sql.DB: %v", err)
	}

	sqlDB.SetConnMaxLifetime(c.config.ConnMaxLifetime)
	sqlDB.SetMaxIdleConns(c.config.MaxIdleConns)
	sqlDB.SetMaxOpenConns(c.config.MaxOpenConns)

	// 测试连接
	if err := sqlDB.Ping(); err != nil {
		return fmt.Errorf("failed to ping ClickHouse: %v", err)
	}

	c.db = db
	return nil
}

// Stop 停止ClickHouse GORM组件
func (c *ClickHouseGORMComponent) Stop(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.db != nil {
		sqlDB, err := c.db.DB()
		if err != nil {
			return fmt.Errorf("failed to get sql.DB: %v", err)
		}
		if err := sqlDB.Close(); err != nil {
			return fmt.Errorf("failed to close ClickHouse connection: %v", err)
		}
		c.db = nil
	}
	return nil
}

// DB 获取GORM DB实例
func (c *ClickHouseGORMComponent) DB() *gorm.DB {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.db
}

// ==================== 默认实例访问方法 ====================

// 获取默认ClickHouse GORM DB
func DefaultClickHouseDB() *gorm.DB {
	if defaultClickHouseGORM == nil {
		panic("default ClickHouse GORM instance not initialized")
	}
	return defaultClickHouseGORM.DB()
}

// ==================== 命名实例访问方法 ====================

// 获取指定名称的ClickHouse GORM DB
func ClickHouseDB(name string) *gorm.DB {
	clickhouseGormMu.RLock()
	instance, ok := clickhouseGormInstances[name]
	clickhouseGormMu.RUnlock()

	if !ok {
		panic(fmt.Sprintf("ClickHouse GORM instance [%s] not found", name))
	}
	return instance.DB()
}

// GetClickHouseGORMComponent 获取指定名称的ClickHouse GORM组件
func GetClickHouseGORMComponent(name string) *ClickHouseGORMComponent {
	clickhouseGormMu.RLock()
	instance, ok := clickhouseGormInstances[name]
	clickhouseGormMu.RUnlock()

	if !ok {
		panic(fmt.Sprintf("ClickHouse GORM instance [%s] not found", name))
	}
	return instance
}
