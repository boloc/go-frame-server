package util

import (
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
)

// TestBuildMysqlDSNHandlesSpecialCharactersInPassword 验证密码含特殊字符时 DSN 仍能被正确解析。
func TestBuildMysqlDSNHandlesSpecialCharactersInPassword(t *testing.T) {
	passwords := []string{
		"12345,./!@#$",
		"p@ss:word/with#many&special?chars=1",
		"包含中文的密码123",
		`back\slash"and'quotes`,
		"", // 空密码也不应该让 DSN 解析报错
	}

	for _, pwd := range passwords {
		t.Run(pwd, func(t *testing.T) {
			dsn := BuildMysqlDSN(MySQLDSNConfig{
				User:     "root",
				Password: pwd,
				Host:     "127.0.0.1",
				Port:     3306,
				Name:     "testdb",
				Charset:  "utf8mb4",
			})

			cfg, err := mysql.ParseDSN(dsn)
			if err != nil {
				t.Fatalf("mysql.ParseDSN(%q) error = %v", dsn, err)
			}
			if cfg.Passwd != pwd {
				t.Errorf("解析回来的密码 = %q, want %q", cfg.Passwd, pwd)
			}
			if cfg.User != "root" || cfg.DBName != "testdb" {
				t.Errorf("解析回来的 User/DBName = %q/%q, 意外被密码里的特殊字符污染了", cfg.User, cfg.DBName)
			}
		})
	}
}

// TestBuildMysqlDSNDefaultsLocToUTC 覆盖 Loc 留空时的默认行为：应该固定退回 UTC，
// 不跟随 time.Local——MySQL 连接的时区约定不该随进程的业务时区设置变化，见
// BuildMysqlDSN 的文档注释。
func TestBuildMysqlDSNDefaultsLocToUTC(t *testing.T) {
	dsn := BuildMysqlDSN(MySQLDSNConfig{
		User: "root", Host: "127.0.0.1", Port: 3306, Name: "testdb",
	})

	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("mysql.ParseDSN(%q) error = %v", dsn, err)
	}
	if cfg.Loc != time.UTC {
		t.Fatalf("解析回来的 Loc = %v, want time.UTC", cfg.Loc)
	}
}

// TestBuildMysqlDSNLocIsIndependentFromProcessTimezone 覆盖解耦本身：即使进程时区被
// SetProcessTimezone 改成了别的时区，Loc 留空时 MySQL 连接仍然应该是 UTC。
func TestBuildMysqlDSNLocIsIndependentFromProcessTimezone(t *testing.T) {
	original := time.Local
	defer func() { time.Local = original }()

	if err := SetProcessTimezone("Asia/Shanghai"); err != nil {
		t.Fatalf("SetProcessTimezone error = %v", err)
	}

	dsn := BuildMysqlDSN(MySQLDSNConfig{
		User: "root", Host: "127.0.0.1", Port: 3306, Name: "testdb",
	})
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("mysql.ParseDSN(%q) error = %v", dsn, err)
	}
	if cfg.Loc != time.UTC {
		t.Fatalf("解析回来的 Loc = %v, want time.UTC（不应该跟随进程时区变成 Asia/Shanghai）", cfg.Loc)
	}
}
