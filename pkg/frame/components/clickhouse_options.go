package components

import (
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
)

// clickhouseConnParams 描述连接 ClickHouse 所需的通用参数，供原生与 GORM 组件共用。
type clickhouseConnParams struct {
	DSN         string // 非空时优先用 DSN，忽略下面的字段
	Address     []string
	Database    string
	Username    string
	Password    string
	Protocol    string // "http" 或者其它（含空字符串）都按 native 协议处理
	DialTimeout time.Duration
	ReadTimeout time.Duration
	Compression *clickhouse.Compression
}

// buildClickHouseOptions 把 clickhouseConnParams 转换成 clickhouse-go/v2 的 *clickhouse.Options。
func buildClickHouseOptions(p clickhouseConnParams) (*clickhouse.Options, error) {
	if p.DSN != "" {
		return clickhouse.ParseDSN(p.DSN)
	}

	protocol := clickhouse.Native
	if p.Protocol == "http" {
		protocol = clickhouse.HTTP
	}

	return &clickhouse.Options{
		Protocol: protocol,
		Addr:     p.Address,
		Auth: clickhouse.Auth{
			Database: p.Database,
			Username: p.Username,
			Password: p.Password,
		},
		DialTimeout: p.DialTimeout,
		ReadTimeout: p.ReadTimeout,
		Compression: p.Compression,
	}, nil
}
