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

	// Timeout 建连（TCP dial）超时，留空默认 5s。没有这个值时，数据库主机不可达会卡到
	// 操作系统的 TCP 超时（通常一两分钟），启动阶段的连接重试和运行期新建连接都会被拖住。
	Timeout string `mapstructure:"timeout"`
	// ReadTimeout/WriteTimeout 单次 I/O 超时，留空默认不限制（由 context 控制查询时长）。
	// 只在确认没有超过这个时长的合法慢查询时才设置。
	ReadTimeout  string `mapstructure:"read_timeout"`
	WriteTimeout string `mapstructure:"write_timeout"`
}

// DefaultMySQLDialTimeout 是 MySQLDSNConfig.Timeout 留空时的建连超时。
const DefaultMySQLDialTimeout = 5 * time.Second

// BuildMysqlDSN 拼接 MySQL DSN，由驱动处理密码中的特殊字符。
// Host/Port/Name/User 缺失、Loc 不是合法时区名、超时字符串解析失败时返回 error——
// 这些都是配置错误，应该让启动失败，而不是静默落到某个默认值后带病运行。
//
// Loc 留空时默认 UTC，跟 SetProcessTimezone/time.Local 无关：MySQL 的 DATETIME 不带
// 时区信息，存的是裸字符串，连接约定应该固定不变，不应该随着进程的业务时区
// （server.timezone，给 time.Now()/日志用的）改了就跟着变，否则同一批历史数据会在
// 时区设置变化后被重新解释成不同的时刻。真的需要某个实例用别的时区，显式配 loc 覆盖。
func BuildMysqlDSN(cfg MySQLDSNConfig) (string, error) {
	switch {
	case cfg.Host == "":
		return "", fmt.Errorf("mysql dsn: host 不能为空")
	case cfg.Port <= 0 || cfg.Port > 65535:
		return "", fmt.Errorf("mysql dsn: port %d 不合法", cfg.Port)
	case cfg.Name == "":
		return "", fmt.Errorf("mysql dsn: name（数据库名）不能为空")
	case cfg.User == "":
		return "", fmt.Errorf("mysql dsn: user 不能为空")
	}

	loc := cfg.Loc
	if loc == "" {
		loc = "UTC"
	}
	location, err := time.LoadLocation(loc)
	if err != nil {
		return "", fmt.Errorf("mysql dsn: loc %q 不是合法的时区名: %w", cfg.Loc, err)
	}

	dialTimeout, err := parseDurationOrDefault("timeout", cfg.Timeout, DefaultMySQLDialTimeout)
	if err != nil {
		return "", err
	}
	readTimeout, err := parseDurationOrDefault("read_timeout", cfg.ReadTimeout, 0)
	if err != nil {
		return "", err
	}
	writeTimeout, err := parseDurationOrDefault("write_timeout", cfg.WriteTimeout, 0)
	if err != nil {
		return "", err
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
		Timeout:              dialTimeout,
		ReadTimeout:          readTimeout,
		WriteTimeout:         writeTimeout,
	}

	if cfg.Collation != "" {
		dsnCfg.Collation = cfg.Collation
	}

	// time_zone：把 MySQL 会话时区也钉到和 loc 一致。loc 只决定 Go 这一侧怎么解释读出来的
	// DATETIME 裸字符串；NOW()/CURRENT_TIMESTAMP 用的是会话 time_zone。两边不一致时，
	// 库自己生成的时间会被 Go 按 loc 误读，差出一个偏移。
	// 会话侧一律写 '+08:00' 这种偏移，不写 Asia/Shanghai：后者要时区表，没装会 1298。
	// loc 仍用 IANA 名，Go 读 DATETIME 的语义不变。
	params := map[string]string{
		"time_zone": mysqlSessionTimeZone(location),
	}
	if cfg.Charset != "" {
		params["charset"] = cfg.Charset
	}
	dsnCfg.Params = params

	return dsnCfg.FormatDSN(), nil
}

// mysqlSessionTimeZone 把 loc 收成 MySQL 一定认的偏移，例如 '+08:00'、'+00:00'。
// 不用 IANA 名，避免没装时区表时 Error 1298。
func mysqlSessionTimeZone(loc *time.Location) string {
	_, offset := time.Now().In(loc).Zone()
	sign := "+"
	if offset < 0 {
		sign = "-"
		offset = -offset
	}
	return fmt.Sprintf("'%s%02d:%02d'", sign, offset/3600, (offset%3600)/60)
}

// parseDurationOrDefault 解析配置里的时长字符串；空串用 def，非法值返回带字段名的 error。
func parseDurationOrDefault(field, value string, def time.Duration) (time.Duration, error) {
	if value == "" {
		return def, nil
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("mysql dsn: %s=%q 不是合法的时长（例如 5s、500ms）: %w", field, value, err)
	}
	return d, nil
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
