package components

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/boloc/go-frame-server/pkg/constant"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

// MySQLConfig MySQL配置
type MySQLConfig struct {
	MasterDSN       string
	SlavesDSN       []string // 支持多个从库
	MaxIdleConns    int
	MaxOpenConns    int
	ConnMaxLifetime time.Duration
	LogLevel        logger.LogLevel
	Prefix          string
}

// applyDefaults 设置默认值
func (c *MySQLConfig) applyDefaults() {
	if c.MaxIdleConns == 0 {
		c.MaxIdleConns = 10
	}
	if c.MaxOpenConns == 0 {
		c.MaxOpenConns = 100
	}
	if c.ConnMaxLifetime == 0 {
		c.ConnMaxLifetime = time.Hour
	}
	if c.LogLevel == 0 {
		c.LogLevel = logger.Info
	}
}

// GormLogLevelForEnv 根据环境变量设置Gorm日志级别
func GormLogLevelForEnv(env string) logger.LogLevel {
	switch env {
	case constant.EnvLocal:
		return logger.Info
	case constant.EnvDev:
		return logger.Warn
	case constant.EnvTest:
		return logger.Warn
	case constant.EnvProd:
		return logger.Error
	case constant.EnvSilent:
		return logger.Silent
	default:
		return logger.Warn
	}
}

// MySQLComponent MySQL组件
type MySQLComponent struct {
	master   *gorm.DB
	replicas []*gorm.DB
	config   *MySQLConfig
	current  uint32 // 使用 uint32 配合 atomic
	mu       sync.RWMutex
}

var (
	mysqlInstances     = make(map[string]*MySQLComponent)
	mysqlInstancesOnce = make(map[string]*sync.Once)
	defaultMySQLDB     *MySQLComponent
	mysqlMu            sync.RWMutex
)

// NewMySQLComponent 创建MySQL组件
func NewMySQLComponent(name string, config *MySQLConfig, isDefault bool) *MySQLComponent {
	mysqlMu.Lock()
	if _, exist := mysqlInstancesOnce[name]; !exist {
		mysqlInstancesOnce[name] = &sync.Once{}
	}
	once := mysqlInstancesOnce[name]
	mysqlMu.Unlock()

	once.Do(func() {
		// 兜底设置默认值
		config.applyDefaults()

		// 创建MySQL组件
		m := &MySQLComponent{
			config: config,
		}

		mysqlMu.Lock()
		mysqlInstances[name] = m
		if isDefault {
			defaultMySQLDB = m
		}
		mysqlMu.Unlock()
	})

	mysqlMu.RLock()
	instance := mysqlInstances[name]
	mysqlMu.RUnlock()

	return instance
}

// Start 启动MySQL组件
func (m *MySQLComponent) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 如果已经连接主库，则跳过
	if m.master != nil {
		return nil
	}

	// 连接主库
	master, err := m.connectDB(m.config.MasterDSN)
	if err != nil {
		return fmt.Errorf("failed to connect to master: %v", err)
	}
	m.master = master

	// 连接从库们
	for _, slaveDSN := range m.config.SlavesDSN {
		replica, err := m.connectDB(slaveDSN)
		if err != nil {
			return fmt.Errorf("failed to connect to slave(%s): %v", slaveDSN, err)
		}
		m.replicas = append(m.replicas, replica)
	}

	return nil
}

// connectDB 连接数据库
func (m *MySQLComponent) connectDB(dsn string) (*gorm.DB, error) {
	gormConfig := &gorm.Config{
		Logger: logger.Default.LogMode(m.config.LogLevel),
	}

	// 判断是否需要前缀
	if m.config.Prefix != "" {
		gormConfig.NamingStrategy = schema.NamingStrategy{
			TablePrefix: m.config.Prefix,
		}
	}

	db, err := gorm.Open(mysql.Open(dsn), gormConfig)
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	sqlDB.SetMaxIdleConns(m.config.MaxIdleConns)
	sqlDB.SetMaxOpenConns(m.config.MaxOpenConns)
	sqlDB.SetConnMaxLifetime(m.config.ConnMaxLifetime)

	if err := sqlDB.Ping(); err != nil {
		return nil, err
	}
	return db, nil
}

// Stop 停止MySQL组件
func (m *MySQLComponent) Stop(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 关闭主库
	if m.master != nil {
		if sqlDB, err := m.master.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}

	// 关闭从库
	for _, slave := range m.replicas {
		if sqlDB, err := slave.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}
	return nil
}

// Master 获取主库连接
func (m *MySQLComponent) Master() *gorm.DB {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.master
}

// Slave 获取从库连接（轮询方式）
func (m *MySQLComponent) Slave() *gorm.DB {
	m.mu.RLock()
	replicaLen := len(m.replicas)
	master := m.master
	m.mu.RUnlock()

	if replicaLen == 0 { // 如果从库数量为 0
		return master // 返回主库
	}

	n := atomic.AddUint32(&m.current, 1)
	return m.replicas[n%uint32(replicaLen)]
}

// ==================== 默认实例访问方法 ====================

// DefaultMasterDB 获取默认实例的主库连接
func DefaultMasterDB() *gorm.DB {
	if defaultMySQLDB == nil {
		panic("default MySQL instance not initialized")
	}
	return defaultMySQLDB.Master()
}

// DefaultSlaveDB 获取默认实例的从库连接
func DefaultSlaveDB() *gorm.DB {
	if defaultMySQLDB == nil {
		panic("default MySQL instance not initialized")
	}
	return defaultMySQLDB.Slave()
}

// ==================== 命名实例访问方法 ====================

// MasterDB 获取指定实例的主库连接
func MasterDB(name string) *gorm.DB {
	mysqlMu.RLock()
	instance, ok := mysqlInstances[name]
	mysqlMu.RUnlock()

	if !ok {
		panic(fmt.Sprintf("MySQL instance [%s] not found", name))
	}
	return instance.Master()
}

// SlaveDB 获取指定实例的从库连接
func SlaveDB(name string) *gorm.DB {
	mysqlMu.RLock()
	instance, ok := mysqlInstances[name]
	mysqlMu.RUnlock()

	if !ok {
		panic(fmt.Sprintf("MySQL instance [%s] not found", name))
	}
	return instance.Slave()
}

// GetMySQLComponent 获取指定MySQL组件实例
func GetMySQLComponent(name string) *MySQLComponent {
	mysqlMu.RLock()
	instance, ok := mysqlInstances[name]
	mysqlMu.RUnlock()

	if !ok {
		panic(fmt.Sprintf("MySQL instance [%s] not found", name))
	}
	return instance
}
