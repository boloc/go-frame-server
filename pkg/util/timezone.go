package util

import (
	"fmt"
	"time"

	_ "time/tzdata" // 内嵌 IANA 时区数据库，避免精简容器镜像缺系统时区文件时加载失败
)

// SetProcessTimezone 把整个进程的默认时区（time.Local）设置为 name；name 为空时用 UTC。
//
// 之后任何没有显式指定时区的代码——time.Now()、日志时间戳、MySQL DSN 里 loc 留空时的
// 默认值（见 BuildMysqlDSN）——都会落到同一个时区。必须在建立数据库连接等依赖
// time.Local 的代码之前调用一次。
func SetProcessTimezone(name string) error {
	if name == "" {
		name = "UTC"
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return fmt.Errorf("加载时区 %q 失败: %w", name, err)
	}
	time.Local = loc
	return nil
}

// FormatLocal 把 t 转换到当前进程时区（time.Local，即 SetProcessTimezone 设置的
// server.timezone）后按 time.DateTime 格式化。
//
// 这是项目里"把一个 time.Time 展示给用户"的统一入口：从数据库读出来的 time.Time
// 携带的时区是数据库连接的 loc（默认 UTC，见 BuildMysqlDSN），不是项目时区，直接对它
// 调 .Format() 会把数据库的时区当成项目时区展示出去；t 为零值（未设置）时返回空字符串。
func FormatLocal(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.In(time.Local).Format(time.DateTime)
}
