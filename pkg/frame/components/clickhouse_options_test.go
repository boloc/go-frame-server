package components

import (
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
)

func TestBuildClickHouseOptionsFromStructuredFields(t *testing.T) {
	options, err := buildClickHouseOptions(clickhouseConnParams{
		Address:     []string{"127.0.0.1:9000"},
		Database:    "testdb",
		Username:    "default",
		Password:    "secret",
		Protocol:    "http",
		DialTimeout: 5 * time.Second,
		ReadTimeout: 10 * time.Second,
		Compression: &clickhouse.Compression{Method: clickhouse.CompressionLZ4},
	})
	if err != nil {
		t.Fatalf("buildClickHouseOptions() error = %v", err)
	}

	if options.Protocol != clickhouse.HTTP {
		t.Errorf("Protocol = %v, want HTTP", options.Protocol)
	}
	if len(options.Addr) != 1 || options.Addr[0] != "127.0.0.1:9000" {
		t.Errorf("Addr = %v, want [127.0.0.1:9000]", options.Addr)
	}
	if options.Auth.Database != "testdb" || options.Auth.Username != "default" || options.Auth.Password != "secret" {
		t.Errorf("Auth = %+v, 字段和输入不匹配", options.Auth)
	}
	if options.DialTimeout != 5*time.Second || options.ReadTimeout != 10*time.Second {
		t.Errorf("DialTimeout/ReadTimeout = %v/%v, want 5s/10s", options.DialTimeout, options.ReadTimeout)
	}
}

func TestBuildClickHouseOptionsDefaultsToNativeProtocol(t *testing.T) {
	options, err := buildClickHouseOptions(clickhouseConnParams{Address: []string{"127.0.0.1:9000"}})
	if err != nil {
		t.Fatalf("buildClickHouseOptions() error = %v", err)
	}
	if options.Protocol != clickhouse.Native {
		t.Errorf("Protocol = %v, want Native（Protocol 字段留空时的默认值）", options.Protocol)
	}
}

// TestBuildClickHouseOptionsDSNTakesPriority 验证 DSN 非空时忽略结构化字段，直接走
// clickhouse.ParseDSN——DSN 是"我已经有一个现成字符串，不想拆成结构化字段"的逃生舱口。
func TestBuildClickHouseOptionsDSNTakesPriority(t *testing.T) {
	options, err := buildClickHouseOptions(clickhouseConnParams{
		DSN:      "clickhouse://user:pass@127.0.0.1:9000/mydb",
		Database: "should-be-ignored",
	})
	if err != nil {
		t.Fatalf("buildClickHouseOptions() error = %v", err)
	}
	if options.Auth.Database != "mydb" {
		t.Errorf("Auth.Database = %q, want %q（应该来自 DSN，不是结构化字段）", options.Auth.Database, "mydb")
	}
}

func TestBuildClickHouseOptionsInvalidDSNReturnsError(t *testing.T) {
	_, err := buildClickHouseOptions(clickhouseConnParams{DSN: "not a valid dsn"})
	if err == nil {
		t.Fatal("expected an error for an invalid DSN")
	}
}
