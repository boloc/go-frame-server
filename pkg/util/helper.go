package util

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/go-sql-driver/mysql"
)

// MySQLDSNConfig 是 BuildMysqlDSN 的参数，可直接用 viper UnmarshalKey 填充。
type MySQLDSNConfig struct {
	User      string `mapstructure:"user"`
	Password  string `mapstructure:"password"`
	Host      string `mapstructure:"host"`
	Port      int    `mapstructure:"port"`
	Name      string `mapstructure:"name"`
	Charset   string `mapstructure:"charset"`   // 可选，如 utf8mb4
	Collation string `mapstructure:"collation"` // 可选，推荐 utf8mb4_0900_ai_ci（MySQL 8.0+）
	Loc       string `mapstructure:"loc"`       // 可选，IANA 时区名（如 Asia/Shanghai）或 Local，默认 UTC
}

// BuildMysqlDSN 拼接 MySQL DSN，由驱动处理密码中的特殊字符。
//
// Loc 留空时默认 UTC，跟 SetProcessTimezone/time.Local 无关：MySQL 的 DATETIME 不带
// 时区信息，存的是裸字符串，连接约定应该固定不变，不应该随着进程的业务时区
// （server.timezone，给 time.Now()/日志用的）改了就跟着变，否则同一批历史数据会在
// 时区设置变化后被重新解释成不同的时刻。真的需要某个实例用别的时区，显式配 loc 覆盖。
func BuildMysqlDSN(cfg MySQLDSNConfig) string {
	loc := cfg.Loc
	if loc == "" {
		loc = "UTC"
	}
	location, err := time.LoadLocation(loc)
	if err != nil {
		location = time.UTC
	}

	dsnCfg := mysql.Config{
		User:                 cfg.User,
		Passwd:               cfg.Password,
		Net:                  "tcp",
		Addr:                 fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		DBName:               cfg.Name,
		ParseTime:            true,
		Loc:                  location,
		AllowNativePasswords: true,
	}

	if cfg.Collation != "" {
		dsnCfg.Collation = cfg.Collation
	}
	if cfg.Charset != "" {
		dsnCfg.Params = map[string]string{"charset": cfg.Charset}
	}

	return dsnCfg.FormatDSN()
}

// TimeToTimestamp 将某个时区下的时间字符串转换为 UTC 时间戳。
func TimeToTimestamp(fromDatetime string, fromTimezone string) (int64, error) {
	location, err := time.LoadLocation(fromTimezone)
	if err != nil {
		return 0, err
	}

	t, err := time.ParseInLocation(time.DateTime, fromDatetime, location)
	if err != nil {
		return 0, err
	}

	return t.UTC().Unix(), nil
}

// RandomTTL 返回 [base, base*(1+jitterRatio)) 内的随机 TTL。jitterRatio <= 0 时返回 base。
func RandomTTL(base time.Duration, jitterRatio float64) time.Duration {
	if jitterRatio <= 0 || base <= 0 {
		return base
	}
	maxJitter := int64(float64(base) * jitterRatio)
	if maxJitter <= 0 {
		return base
	}
	return base + time.Duration(rand.Int63n(maxJitter))
}
