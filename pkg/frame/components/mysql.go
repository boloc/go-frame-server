package components

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/boloc/go-frame-server/pkg/constant"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

// MySQLConfig MySQL配置。
// MasterDSN/SlavesDSN 在 Start 时连一次后固定；主从切换需由 VIP/Proxy 等基础设施完成。
// 若 DSN 写物理 IP，主库漂移后需改配置并 SIGHUP 热重启。
type MySQLConfig struct {
	MasterDSN       string
	SlavesDSN       []string // 支持多个从库
	MaxIdleConns    int
	MaxOpenConns    int
	ConnMaxLifetime time.Duration
	// ConnMaxIdleTime 空闲超过该时间后从池中剔除。默认应明显小于 MySQL wait_timeout。
	ConnMaxIdleTime time.Duration
	LogLevel        logger.LogLevel
	Prefix          string

	// ConnectRetryAttempts 启动时连接失败的重试次数（含第一次尝试），默认 3。
	ConnectRetryAttempts int
	// ConnectRetryInterval 每次重试之间的等待时间，默认 2s。
	ConnectRetryInterval time.Duration
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
	if c.ConnMaxIdleTime == 0 {
		c.ConnMaxIdleTime = 10 * time.Minute
	}
	if c.LogLevel == 0 {
		c.LogLevel = logger.Info
	}
	if c.ConnectRetryAttempts == 0 {
		c.ConnectRetryAttempts = 3
	}
	if c.ConnectRetryInterval == 0 {
		c.ConnectRetryInterval = 2 * time.Second
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
	mysqlInstances map[string]*MySQLComponent = make(map[string]*MySQLComponent)
	defaultMySQLDB *MySQLComponent
	mysqlMu        sync.RWMutex
)

// NewMySQLComponent 创建MySQL组件。同一 name 只能注册一次，重复注册会 panic。
func NewMySQLComponent(name string, config *MySQLConfig, isDefault bool) *MySQLComponent {
	mysqlMu.Lock()
	defer mysqlMu.Unlock()

	if _, exists := mysqlInstances[name]; exists {
		panic(fmt.Sprintf("MySQL instance [%s] already registered: NewMySQLComponent 不能对同一个 name 调用两次", name))
	}

	config.applyDefaults()
	m := &MySQLComponent{config: config}
	mysqlInstances[name] = m
	if isDefault {
		defaultMySQLDB = m
	}
	return m
}

// Start 启动MySQL组件。主库连接失败返回 error；从库连接失败会跳过该从库继续启动。
func (m *MySQLComponent) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Start 幂等：已连接主库则跳过。
	if m.master != nil {
		return nil
	}

	master, err := m.connectWithRetry(ctx, m.config.MasterDSN)
	if err != nil {
		return fmt.Errorf("failed to connect to master: %w", err)
	}
	m.master = master

	var replicas []*gorm.DB
	for _, slaveDSN := range m.config.SlavesDSN {
		replica, err := m.connectWithRetry(ctx, slaveDSN)
		if err != nil {
			fmt.Printf("[mysql] 警告：从库连接失败，跳过该从库继续启动: dsn=%s err=%v\n", maskDSN(slaveDSN), err)
			continue
		}
		replicas = append(replicas, replica)
	}
	m.replicas = replicas

	return nil
}

// connectWithRetry 按 ConnectRetryAttempts/ConnectRetryInterval 重试连接。
func (m *MySQLComponent) connectWithRetry(ctx context.Context, dsn string) (*gorm.DB, error) {
	var db *gorm.DB
	err := retryConnect(ctx, m.config.ConnectRetryAttempts, m.config.ConnectRetryInterval, func() error {
		var connectErr error
		db, connectErr = m.connectDB(dsn)
		return connectErr
	})
	if err != nil {
		return nil, err
	}
	return db, nil
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
	sqlDB.SetConnMaxIdleTime(m.config.ConnMaxIdleTime)

	if err := sqlDB.Ping(); err != nil {
		_ = sqlDB.Close()
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
		m.master = nil
	}

	// 关闭从库
	for _, slave := range m.replicas {
		if sqlDB, err := slave.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}
	m.replicas = nil
	return nil
}

// Master 获取主库连接
func (m *MySQLComponent) Master() *gorm.DB {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.master
}

// Slave 轮询获取从库连接；无可用从库时回落到主库。
//
// 注意：锁内拷贝的是 replicas 这个 slice header 本身，不是只拷贝长度——否则并发的
// Stop() 把 m.replicas 置 nil 后，锁外再用"过期"的长度去下标访问会 panic。
func (m *MySQLComponent) Slave() *gorm.DB {
	m.mu.RLock()
	replicas := m.replicas
	master := m.master
	m.mu.RUnlock()

	if len(replicas) == 0 { // 如果从库数量为 0
		return master // 返回主库
	}

	n := atomic.AddUint32(&m.current, 1)
	return replicas[n%uint32(len(replicas))]
}

// poolStats 返回主库和各从库连接池快照。replicas 顺序与 Slave 轮询一致。
func (m *MySQLComponent) poolStats() (master *sql.DBStats, replicas []sql.DBStats) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.master != nil {
		if sqlDB, err := m.master.DB(); err == nil {
			stats := sqlDB.Stats()
			master = &stats
		}
	}

	for _, replica := range m.replicas {
		sqlDB, err := replica.DB()
		if err != nil {
			continue
		}
		replicas = append(replicas, sqlDB.Stats())
	}
	return master, replicas
}

// maskDSN 脱敏 DSN 中的密码（user:password@tcp(...) 格式）。
func maskDSN(dsn string) string {
	at := strings.Index(dsn, "@")
	colon := strings.Index(dsn, ":")
	if at <= 0 || colon <= 0 || colon >= at {
		return dsn
	}
	return dsn[:colon+1] + "***" + dsn[at:]
}

// ==================== 默认实例访问方法 ====================
//
// panic 版访问器适合启动阶段；运行时请用对应的 Try 版本。
//
// 下面这一组 Try* 的 ok 语义是"这个连接现在能不能用"，不是"这个名字有没有被注册过"：
// 组件被 Stop() 之后，实例还在全局注册表里，但 Master()/Slave() 会返回 nil，这里多判
// 一次 db != nil，避免 ok=true 但 db 是 nil 的假阳性。

// TryDefaultMasterDB 获取默认实例的主库连接；未注册或当前不可用时返回 (nil, false)。
func TryDefaultMasterDB() (*gorm.DB, bool) {
	mysqlMu.RLock()
	instance := defaultMySQLDB
	mysqlMu.RUnlock()

	if instance == nil {
		return nil, false
	}
	db := instance.Master()
	return db, db != nil
}

// DefaultMasterDB 获取默认实例的主库连接；未注册或当前不可用时 panic。运行时请用 TryDefaultMasterDB。
func DefaultMasterDB() *gorm.DB {
	db, ok := TryDefaultMasterDB()
	if !ok {
		panic("default MySQL instance not initialized or not currently connected")
	}
	return db
}

// TryDefaultSlaveDB 获取默认实例的从库连接；未注册或当前不可用时返回 (nil, false)。
func TryDefaultSlaveDB() (*gorm.DB, bool) {
	mysqlMu.RLock()
	instance := defaultMySQLDB
	mysqlMu.RUnlock()

	if instance == nil {
		return nil, false
	}
	db := instance.Slave()
	return db, db != nil
}

// DefaultSlaveDB 获取默认实例的从库连接；未注册或当前不可用时 panic。运行时请用 TryDefaultSlaveDB。
func DefaultSlaveDB() *gorm.DB {
	db, ok := TryDefaultSlaveDB()
	if !ok {
		panic("default MySQL instance not initialized or not currently connected")
	}
	return db
}

// ==================== 命名实例访问方法 ====================

// TryMasterDB 获取指定实例的主库连接；实例不存在或当前不可用时返回 (nil, false)。
func TryMasterDB(name string) (*gorm.DB, bool) {
	instance, ok := TryGetMySQLComponent(name)
	if !ok {
		return nil, false
	}
	db := instance.Master()
	return db, db != nil
}

// MasterDB 获取指定实例的主库连接；实例不存在或当前不可用时 panic。运行时请用 TryMasterDB。
func MasterDB(name string) *gorm.DB {
	db, ok := TryMasterDB(name)
	if !ok {
		panic(fmt.Sprintf("MySQL instance [%s] not found or not currently connected", name))
	}
	return db
}

// TrySlaveDB 获取指定实例的从库连接；实例不存在或当前不可用时返回 (nil, false)。
func TrySlaveDB(name string) (*gorm.DB, bool) {
	instance, ok := TryGetMySQLComponent(name)
	if !ok {
		return nil, false
	}
	db := instance.Slave()
	return db, db != nil
}

// SlaveDB 获取指定实例的从库连接；实例不存在或当前不可用时 panic。运行时请用 TrySlaveDB。
func SlaveDB(name string) *gorm.DB {
	db, ok := TrySlaveDB(name)
	if !ok {
		panic(fmt.Sprintf("MySQL instance [%s] not found or not currently connected", name))
	}
	return db
}

// TryGetMySQLComponent 获取指定MySQL组件实例；不存在时返回 (nil, false)，不 panic。
func TryGetMySQLComponent(name string) (*MySQLComponent, bool) {
	mysqlMu.RLock()
	defer mysqlMu.RUnlock()
	instance, ok := mysqlInstances[name]
	return instance, ok
}

// GetMySQLComponent 获取指定MySQL组件实例；不存在时 panic。运行时请用 TryGetMySQLComponent。
func GetMySQLComponent(name string) *MySQLComponent {
	instance, ok := TryGetMySQLComponent(name)
	if !ok {
		panic(fmt.Sprintf("MySQL instance [%s] not found", name))
	}
	return instance
}
