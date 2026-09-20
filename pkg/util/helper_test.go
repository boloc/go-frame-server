package util

import (
	"strings"
	"testing"
	"time"
)

func TestMysqlSessionTimeZone(t *testing.T) {
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	if got := mysqlSessionTimeZone(shanghai); got != "'+08:00'" {
		t.Fatalf("Asia/Shanghai session tz = %s, want '+08:00'", got)
	}
	if got := mysqlSessionTimeZone(time.UTC); got != "'+00:00'" {
		t.Fatalf("UTC session tz = %s, want '+00:00'", got)
	}
}

func TestBuildMysqlDSNSessionTimeZoneOffset(t *testing.T) {
	dsn, err := BuildMysqlDSN(MySQLDSNConfig{
		User:     "root",
		Password: "root",
		Host:     "127.0.0.1",
		Port:     3306,
		Name:     "nav-market",
		Charset:  "utf8mb4",
		Loc:      "Asia/Shanghai",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(dsn, "time_zone=") || strings.Contains(dsn, "time_zone=%27Asia") {
		t.Fatalf("dsn should use numeric session time_zone, got %s", dsn)
	}
}
