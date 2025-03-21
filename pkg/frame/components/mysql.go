package components

import (
	"context"
	"fmt"
	"frame-server/pkg/constant"
	"sync"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

// MySQLConfig MySQL配置
type MySQLConfig struct {
	MasterDSN       string
	SlaveDSNs       []string // 修改为切片，支持多个从库
	MaxIdleConns    int
	MaxOpenConns    int
	ConnMaxLifetime time.Duration
	LogLevel        logger.LogLevel
	Prefix          string
}

// GormLogLevelForEnv 根据环境变量设置Gorm日志级别
func GormLogLevelForEnv(env string) logger.LogLevel {
	switch env {
	case constant.EnvLocal:
		return logger.Info
	case constant.EnvTest:
		return logger.Error
	case constant.EnvProd:
		return logger.Error
	default:
		return logger.Error
	}
}

// MySQLComponent MySQL组件
type MySQLComponent struct {
	master   *gorm.DB
	replicas []*gorm.DB
	config   *MySQLConfig
	current  int
	mu       sync.RWMutex
}

var (
	mysqlInstances = make(map[string]*MySQLComponent)
	DefaultDB      *MySQLComponent // 添加默认实例
	mu             sync.RWMutex
)

// NewMySQLComponent 创建MySQL组件
func NewMySQLComponent(name string, config *MySQLConfig, isDefault bool) *MySQLComponent {
	if config.MaxIdleConns == 0 {
		config.MaxIdleConns = 10
	}
	if config.MaxOpenConns == 0 {
		config.MaxOpenConns = 100
	}
	if config.ConnMaxLifetime == 0 {
		config.ConnMaxLifetime = time.Hour
	}
	if config.LogLevel == 0 {
		config.LogLevel = logger.Info
	}

	m := &MySQLComponent{
		config: config,
	}

	mu.Lock()
	mysqlInstances[name] = m
	if isDefault {
		DefaultDB = m
	}
	mu.Unlock()

	return m
}

// Start 启动MySQL组件
func (m *MySQLComponent) Start(ctx context.Context) error {
	// 连接主库
	master, err := m.connectDB(m.config.MasterDSN)
	if err != nil {
		return fmt.Errorf("failed to connect to master: %v", err)
	}
	m.master = master

	// 连接从库们
	for _, slaveDSN := range m.config.SlaveDSNs {
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
	return m.master
}

// Replica 获取从库连接（轮询方式）
func (m *MySQLComponent) Slave() *gorm.DB {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.replicas) == 0 {
		return m.master
	}

	m.current = (m.current + 1) % len(m.replicas)
	return m.replicas[m.current]
}

// 默认实例的全局访问方法
func GetDefaultDB() *gorm.DB {
	// 获取这个结构体内所有参数
	if DefaultDB == nil {
		panic("default MySQL instance not initialized")
	}
	return DefaultDB.Master()
}

// 默认实例的从库访问方法
func DefaultSlaveDB() *gorm.DB {
	if DefaultDB == nil {
		panic("default MySQL instance not initialized")
	}
	return DefaultDB.Slave()
}

// 保留原有的命名实例访问方法
func DB(name string) *gorm.DB {
	mu.RLock()
	instance, ok := mysqlInstances[name]
	mu.RUnlock()

	if !ok {
		panic(fmt.Sprintf("MySQL instance [%s] not found", name))
	}
	return instance.Master()
}

// ReplicaDB 获取从库连接
func SlaveDB(name string) *gorm.DB {
	mu.RLock()
	instance, ok := mysqlInstances[name]
	mu.RUnlock()

	if !ok {
		panic(fmt.Sprintf("MySQL instance [%s] not found", name))
	}
	return instance.Slave()
}

// 获取指定实例
func GetMySQLComponent(name string) *MySQLComponent {
	mu.RLock()
	instance, ok := mysqlInstances[name]
	mu.RUnlock()

	if !ok {
		panic(fmt.Sprintf("MySQL instance [%s] not found", name))
	}
	return instance
}
