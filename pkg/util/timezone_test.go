package util

import (
	"testing"
	"time"
)

// TestSetProcessTimezoneDefaultsToUTCWhenEmpty 验证未配置时区时默认落到 UTC。
func TestSetProcessTimezoneDefaultsToUTCWhenEmpty(t *testing.T) {
	original := time.Local
	defer func() { time.Local = original }()

	if err := SetProcessTimezone(""); err != nil {
		t.Fatalf("SetProcessTimezone(\"\") error = %v", err)
	}
	if time.Local.String() != "UTC" {
		t.Fatalf("time.Local = %v, want UTC", time.Local)
	}
}

// TestSetProcessTimezoneLoadsNamedZone 验证能加载一个具体的 IANA 时区。
func TestSetProcessTimezoneLoadsNamedZone(t *testing.T) {
	original := time.Local
	defer func() { time.Local = original }()

	if err := SetProcessTimezone("Asia/Shanghai"); err != nil {
		t.Fatalf("SetProcessTimezone(\"Asia/Shanghai\") error = %v", err)
	}

	_, offset := time.Now().Zone()
	if offset != 8*3600 {
		t.Fatalf("offset = %d, want 8h（Asia/Shanghai 全年 UTC+8，不受夏令时影响）", offset)
	}
}

// TestSetProcessTimezoneRejectsUnknownZone 验证非法时区名返回 error，不会 panic。
func TestSetProcessTimezoneRejectsUnknownZone(t *testing.T) {
	original := time.Local
	defer func() { time.Local = original }()

	if err := SetProcessTimezone("Not/AZone"); err == nil {
		t.Fatal("SetProcessTimezone 对不存在的时区名应该返回 error")
	}
}

// TestFormatLocalConvertsToProcessTimezone 验证 FormatLocal 会先转换到 time.Local
// 再格式化，不是直接格式化 t 自带的时区。
func TestFormatLocalConvertsToProcessTimezone(t *testing.T) {
	original := time.Local
	defer func() { time.Local = original }()

	if err := SetProcessTimezone("Asia/Tokyo"); err != nil {
		t.Fatalf("SetProcessTimezone error = %v", err)
	}

	shanghai, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("LoadLocation error = %v", err)
	}
	// 上海时间 15:00，等价的东京时间是 16:00（东京比上海快 1 小时）。
	shanghaiTime := time.Date(2026, 8, 15, 15, 0, 0, 0, shanghai)

	got := FormatLocal(shanghaiTime)
	want := "2026-08-15 16:00:00"
	if got != want {
		t.Fatalf("FormatLocal() = %q, want %q", got, want)
	}
}

// TestFormatLocalReturnsEmptyForZeroValue 验证零值时间格式化成空字符串，不是
// "0001-01-01 00:00:00"。
func TestFormatLocalReturnsEmptyForZeroValue(t *testing.T) {
	if got := FormatLocal(time.Time{}); got != "" {
		t.Fatalf("FormatLocal(零值) = %q, want 空字符串", got)
	}
}
