package components

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/boloc/go-frame-server/v2/pkg/alert"
	"github.com/boloc/go-frame-server/v2/pkg/constant"
	flog "github.com/boloc/go-frame-server/v2/pkg/logger"

	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

// MySQLConfig MySQL配置。
// MasterDSN/SlavesDSN 在构造时固定；主从切换需由 VIP/Proxy/DNS 等基础设施完成。
// 注意 SIGHUP 热重启只是对同一份配置 Stop 再 Start，不会重新读配置文件，改了 DSN 必须
// 重启进程才生效。
type MySQLConfig struct {
	MasterDSN       string
	SlavesDSN       []string // 支持多个从库
	MaxIdleConns    int
	MaxOpenConns    int
	ConnMaxLifetime time.Duration
	// ConnMaxIdleTime 空闲超过该时间后从池中剔除。默认应明显小于 MySQL wait_timeout。
	ConnMaxIdleTime time.Duration
	// LogLevel GORM 日志级别。SQL 日志经 pkg/logger（zap）落地，见 newGormLogger。
	LogLevel logger.LogLevel
	// SlowThreshold 慢查询阈值，超过即以 Warn 记录（任何 LogLevel >= Warn 时生效），默认 200ms。
	SlowThreshold time.Duration
	Prefix        string

	// ConnectRetryAttempts 启动时连接失败的重试次数（含第一次尝试），默认 3。
	ConnectRetryAttempts int
	// ConnectRetryInterval 每次重试之间的等待时间，默认 2s。
	ConnectRetryInterval time.Duration

	// SkipDefaultTransaction 控制是否关闭 GORM 对单条 Create/Update/Delete 的隐式事务包装。
	// 默认 true（关闭）：单语句写操作不会再多付一次 BEGIN/COMMIT 往返，也不会在网络抖动/
	// 超时时出现"驱动内部已结束事务，GORM defer 又二次 commit/rollback"这种误导性报错。
	// 跨语句的原子性要求必须显式 db.Transaction(...)，不受这个默认值影响。
	// 显式传 false 可以恢复 GORM 原生行为（依赖"带关联的 Create 失败自动回滚"这类语义时使用）。
	SkipDefaultTransaction *bool
}

// applyDefaults 设置默认值
func (c *MySQLConfig) applyDefaults() {
	if c.MaxIdleConns == 0 {
		// 25/50 而不是等比例调大：MaxIdleConns 应该对齐稳态并发量级，MaxOpenConns 是
		// 信号量式的硬顶，不是吞吐量旋钮；1:10 的旧比例（10/100）会导致并发峰值过后归还连接
		// 时大量被直接关闭（MaxIdleClosed 指标飙升），下一波并发又要重新建连。
		c.MaxIdleConns = 25
	}
	if c.MaxOpenConns == 0 {
		c.MaxOpenConns = 50
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
	if c.SlowThreshold == 0 {
		c.SlowThreshold = defaultSlowThreshold
	}
	if c.ConnectRetryAttempts == 0 {
		c.ConnectRetryAttempts = 3
	}
	if c.ConnectRetryInterval == 0 {
		c.ConnectRetryInterval = 2 * time.Second
	}
	if c.SkipDefaultTransaction == nil {
		skip := true
		c.SkipDefaultTransaction = &skip
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
	current  atomic.Uint32 // 使用 uint32 配合 atomic
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
	for i, slaveDSN := range m.config.SlavesDSN {
		replica, err := m.connectWithRetry(ctx, slaveDSN)
		if err != nil {
			// 从库连不上不阻断启动（读会回落到主库），但必须走正式日志 + 告警：
			// 否则这个降级只在 stdout 里一闪而过，线上主库负载翻倍却没人知道原因。
			flog.Warn("mysql: 从库连接失败，跳过该从库继续启动，读请求将回落到主库",
				zap.Int("slave_index", i), zap.String("dsn", maskDSN(slaveDSN)), zap.Error(err))
			alert.Notify(ctx, alert.Event{
				Scope: "mysql", Name: "slave_connect", Message: "slave connect failed, skipped", Err: err,
				Fields: map[string]any{"slave_index": i, "dsn": maskDSN(slaveDSN)},
			})
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
		db, connectErr = m.connectDB(ctx, dsn)
		return connectErr
	})
	if err != nil {
		return nil, err
	}
	return db, nil
}

// connectDB 连接数据库
func (m *MySQLComponent) connectDB(ctx context.Context, dsn string) (*gorm.DB, error) {
	gormConfig := &gorm.Config{
		Logger:                 newGormLogger("mysql", m.config.LogLevel, m.config.SlowThreshold),
		SkipDefaultTransaction: m.config.SkipDefaultTransaction == nil || *m.config.SkipDefaultTransaction,
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

	// 带 ctx 的 Ping：Frame 启动被取消时能立刻退出，不等驱动自己的超时。
	if err := sqlDB.PingContext(ctx); err != nil {
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

	n := m.current.Add(1)
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
// panic 版访问器适合启动阶段；健康检查/可选依赖请用对应的 Try 版本。
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

// DefaultMasterDB 获取默认实例的主库连接；未注册或当前不可用时 panic。健康检查/可选依赖请用 TryDefaultMasterDB。
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

// DefaultSlaveDB 获取默认实例的从库连接；未注册或当前不可用时 panic。健康检查/可选依赖请用 TryDefaultSlaveDB。
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

// MasterDB 获取指定实例的主库连接；实例不存在或当前不可用时 panic。健康检查/可选依赖请用 TryMasterDB。
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

// SlaveDB 获取指定实例的从库连接；实例不存在或当前不可用时 panic。健康检查/可选依赖请用 TrySlaveDB。
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

// GetMySQLComponent 获取指定MySQL组件实例；不存在时 panic。健康检查/可选依赖请用 TryGetMySQLComponent。
func GetMySQLComponent(name string) *MySQLComponent {
	instance, ok := TryGetMySQLComponent(name)
	if !ok {
		panic(fmt.Sprintf("MySQL instance [%s] not found", name))
	}
	return instance
}
