package components

// 连接池指标采集：MySQL 多个命名实例共用一个 Collector，用 instance 标签区分。

import (
	"database/sql"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
)

var (
	mysqlPoolDescs = struct {
		maxOpen, open, inUse, idle                                *prometheus.Desc
		waitCount, waitDuration, maxIdleClosed, maxIdleTimeClosed *prometheus.Desc
		maxLifetimeClosed                                         *prometheus.Desc
	}{
		maxOpen: prometheus.NewDesc("mysql_pool_max_open_connections", "连接池允许的最大连接数（0 表示不限制）",
			[]string{"instance", "role", "index"}, nil),
		open: prometheus.NewDesc("mysql_pool_open_connections", "当前已建立的连接数（使用中+空闲）",
			[]string{"instance", "role", "index"}, nil),
		inUse: prometheus.NewDesc("mysql_pool_in_use_connections", "当前正在被使用的连接数",
			[]string{"instance", "role", "index"}, nil),
		idle: prometheus.NewDesc("mysql_pool_idle_connections", "当前空闲的连接数",
			[]string{"instance", "role", "index"}, nil),
		waitCount: prometheus.NewDesc("mysql_pool_wait_count_total", "累计等待获取连接的次数（持续增长说明池子偏小）",
			[]string{"instance", "role", "index"}, nil),
		waitDuration: prometheus.NewDesc("mysql_pool_wait_duration_seconds_total", "累计等待获取连接花费的时间（秒）",
			[]string{"instance", "role", "index"}, nil),
		maxIdleClosed: prometheus.NewDesc("mysql_pool_max_idle_closed_total", "因超过 MaxIdleConns 被关闭的连接数",
			[]string{"instance", "role", "index"}, nil),
		maxIdleTimeClosed: prometheus.NewDesc("mysql_pool_max_idle_time_closed_total", "因超过 ConnMaxIdleTime 被关闭的连接数",
			[]string{"instance", "role", "index"}, nil),
		maxLifetimeClosed: prometheus.NewDesc("mysql_pool_max_lifetime_closed_total", "因超过 ConnMaxLifetime 被关闭的连接数",
			[]string{"instance", "role", "index"}, nil),
	}

	redisPoolDescs = struct {
		hits, misses, timeouts, totalConns, idleConns, staleConns *prometheus.Desc
	}{
		hits: prometheus.NewDesc("redis_pool_hits_total", "累计从连接池成功复用到空闲连接的次数",
			[]string{"instance"}, nil),
		misses: prometheus.NewDesc("redis_pool_misses_total", "累计没有空闲连接、需要新建连接的次数",
			[]string{"instance"}, nil),
		timeouts: prometheus.NewDesc("redis_pool_timeouts_total", "累计获取连接超时的次数",
			[]string{"instance"}, nil),
		totalConns: prometheus.NewDesc("redis_pool_total_connections", "当前连接池里的连接总数",
			[]string{"instance"}, nil),
		idleConns: prometheus.NewDesc("redis_pool_idle_connections", "当前连接池里的空闲连接数",
			[]string{"instance"}, nil),
		staleConns: prometheus.NewDesc("redis_pool_stale_connections_total", "累计被判定为失效（stale）并移除的连接数",
			[]string{"instance"}, nil),
	}
)

// mysqlPoolCollector 采集多个命名 MySQL 实例的主库 + 从库连接池指标。
// 必须只注册一份：同一套 mysql_pool_* Desc 注册两次会 panic。
type mysqlPoolCollector struct {
	instances map[string]*MySQLComponent
}

// NewMySQLPoolCollector 为全部命名 MySQL 实例创建一份 prometheus.Collector。
// map 的 key 会打到 instance 标签，建议与 NewMySQLComponent 的 name 一致。
func NewMySQLPoolCollector(instances map[string]*MySQLComponent) prometheus.Collector {
	copied := make(map[string]*MySQLComponent, len(instances))
	for name, m := range instances {
		if m != nil {
			copied[name] = m
		}
	}
	return &mysqlPoolCollector{instances: copied}
}

func (c *mysqlPoolCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- mysqlPoolDescs.maxOpen
	ch <- mysqlPoolDescs.open
	ch <- mysqlPoolDescs.inUse
	ch <- mysqlPoolDescs.idle
	ch <- mysqlPoolDescs.waitCount
	ch <- mysqlPoolDescs.waitDuration
	ch <- mysqlPoolDescs.maxIdleClosed
	ch <- mysqlPoolDescs.maxIdleTimeClosed
	ch <- mysqlPoolDescs.maxLifetimeClosed
}

func (c *mysqlPoolCollector) Collect(ch chan<- prometheus.Metric) {
	for name, m := range c.instances {
		master, replicas := m.poolStats()
		if master != nil {
			c.collectOne(ch, name, "master", "0", *master)
		}
		for i, stats := range replicas {
			c.collectOne(ch, name, "slave", strconv.Itoa(i), stats)
		}
	}
}

func (c *mysqlPoolCollector) collectOne(ch chan<- prometheus.Metric, instance, role, index string, s sql.DBStats) {
	labels := []string{instance, role, index}
	ch <- prometheus.MustNewConstMetric(mysqlPoolDescs.maxOpen, prometheus.GaugeValue, float64(s.MaxOpenConnections), labels...)
	ch <- prometheus.MustNewConstMetric(mysqlPoolDescs.open, prometheus.GaugeValue, float64(s.OpenConnections), labels...)
	ch <- prometheus.MustNewConstMetric(mysqlPoolDescs.inUse, prometheus.GaugeValue, float64(s.InUse), labels...)
	ch <- prometheus.MustNewConstMetric(mysqlPoolDescs.idle, prometheus.GaugeValue, float64(s.Idle), labels...)
	ch <- prometheus.MustNewConstMetric(mysqlPoolDescs.waitCount, prometheus.CounterValue, float64(s.WaitCount), labels...)
	ch <- prometheus.MustNewConstMetric(mysqlPoolDescs.waitDuration, prometheus.CounterValue, s.WaitDuration.Seconds(), labels...)
	ch <- prometheus.MustNewConstMetric(mysqlPoolDescs.maxIdleClosed, prometheus.CounterValue, float64(s.MaxIdleClosed), labels...)
	ch <- prometheus.MustNewConstMetric(mysqlPoolDescs.maxIdleTimeClosed, prometheus.CounterValue, float64(s.MaxIdleTimeClosed), labels...)
	ch <- prometheus.MustNewConstMetric(mysqlPoolDescs.maxLifetimeClosed, prometheus.CounterValue, float64(s.MaxLifetimeClosed), labels...)
}

// RedisPoolStatsProvider 由单机/集群/哨兵 Redis 组件实现。
type RedisPoolStatsProvider interface {
	PoolStats() *redis.PoolStats
}

type redisPoolCollector struct {
	instance string
	provider RedisPoolStatsProvider
}

// NewRedisPoolCollector 为指定 Redis 组件（单机/集群/哨兵）创建 prometheus.Collector。
func NewRedisPoolCollector(instance string, provider RedisPoolStatsProvider) prometheus.Collector {
	return &redisPoolCollector{instance: instance, provider: provider}
}

func (c *redisPoolCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- redisPoolDescs.hits
	ch <- redisPoolDescs.misses
	ch <- redisPoolDescs.timeouts
	ch <- redisPoolDescs.totalConns
	ch <- redisPoolDescs.idleConns
	ch <- redisPoolDescs.staleConns
}

func (c *redisPoolCollector) Collect(ch chan<- prometheus.Metric) {
	stats := c.provider.PoolStats()
	if stats == nil {
		// 尚未 Start 或已 Stop 时不产出指标。
		return
	}

	ch <- prometheus.MustNewConstMetric(redisPoolDescs.hits, prometheus.CounterValue, float64(stats.Hits), c.instance)
	ch <- prometheus.MustNewConstMetric(redisPoolDescs.misses, prometheus.CounterValue, float64(stats.Misses), c.instance)
	ch <- prometheus.MustNewConstMetric(redisPoolDescs.timeouts, prometheus.CounterValue, float64(stats.Timeouts), c.instance)
	ch <- prometheus.MustNewConstMetric(redisPoolDescs.totalConns, prometheus.GaugeValue, float64(stats.TotalConns), c.instance)
	ch <- prometheus.MustNewConstMetric(redisPoolDescs.idleConns, prometheus.GaugeValue, float64(stats.IdleConns), c.instance)
	ch <- prometheus.MustNewConstMetric(redisPoolDescs.staleConns, prometheus.CounterValue, float64(stats.StaleConns), c.instance)
}
