package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestResolvePriority(t *testing.T) {
	const defaultPath = "./config/default.yml"

	t.Run("falls back to default when nothing is set", func(t *testing.T) {
		os.Args = []string{"app"}
		t.Setenv("CONFIG_FILE", "")
		if got := Resolve(defaultPath); got != defaultPath {
			t.Fatalf("Resolve() = %q, want %q", got, defaultPath)
		}
	})

	t.Run("env var overrides default", func(t *testing.T) {
		os.Args = []string{"app"}
		t.Setenv("CONFIG_FILE", "./config/from-env.yml")
		if got := Resolve(defaultPath); got != "./config/from-env.yml" {
			t.Fatalf("Resolve() = %q, want %q", got, "./config/from-env.yml")
		}
	})

	t.Run("-c flag overrides env var and default", func(t *testing.T) {
		os.Args = []string{"app", "-c", "./config/from-flag.yml"}
		t.Setenv("CONFIG_FILE", "./config/from-env.yml")
		if got := Resolve(defaultPath); got != "./config/from-flag.yml" {
			t.Fatalf("Resolve() = %q, want %q", got, "./config/from-flag.yml")
		}
	})

	t.Run("--config=value form is supported", func(t *testing.T) {
		os.Args = []string{"app", "--config=./config/from-long-flag.yml"}
		t.Setenv("CONFIG_FILE", "")
		if got := Resolve(defaultPath); got != "./config/from-long-flag.yml" {
			t.Fatalf("Resolve() = %q, want %q", got, "./config/from-long-flag.yml")
		}
	})

	t.Run("does not touch the global flag package", func(t *testing.T) {
		// Resolve 只扫描 os.Args，不应注册或消费全局 flag。
		os.Args = []string{"app", "-c", "./config/from-flag.yml", "-other=1"}
		_ = Resolve(defaultPath)
	})
}

func TestLoadFileReturnsErrorInsteadOfPanicking(t *testing.T) {
	_, err := LoadFile("./this-file-does-not-exist.yml")
	if err == nil {
		t.Fatal("expected LoadFile() to return an error for a missing file")
	}
}

func TestParseDurationEmptyStringMeansNotConfigured(t *testing.T) {
	d, err := ParseDuration("")
	if err != nil {
		t.Fatalf("ParseDuration(\"\") error = %v, want nil", err)
	}
	if d != 0 {
		t.Fatalf("ParseDuration(\"\") = %v, want 0", d)
	}
}

func TestParseDurationValidValue(t *testing.T) {
	d, err := ParseDuration("1h30m")
	if err != nil {
		t.Fatalf("ParseDuration() error = %v", err)
	}
	if d != 90*time.Minute {
		t.Fatalf("ParseDuration() = %v, want 90m", d)
	}
}

// TestParseDurationInvalidValueReturnsError 验证无法解析的非空字符串返回 error，而不是静默 0。
func TestParseDurationInvalidValueReturnsError(t *testing.T) {
	_, err := ParseDuration("not-a-duration")
	if err == nil {
		t.Fatal("ParseDuration() 对无法解析的非空字符串应该返回 error，不应该静默返回 0")
	}
}

// loadYAMLValue 写临时 yml 并读出指定 key。
func loadYAMLValue(t *testing.T, yamlContent, key string) (value string, err error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.yml")
	if err := os.WriteFile(path, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("写临时配置文件失败: %v", err)
	}

	conf, err := LoadFile(path)
	if err != nil {
		return "", err
	}
	return conf.GetString(key), nil
}

// TestYAMLPasswordWithSpecialCharsMustBeQuoted 验证带特殊字符的 YAML 值必须加引号，否则会解析失败或被截断。
func TestYAMLPasswordWithSpecialCharsMustBeQuoted(t *testing.T) {
	t.Run("unquoted value starting with ! fails to parse at all", func(t *testing.T) {
		_, err := loadYAMLValue(t, "db:\n  password: !@#$abc\n", "db.password")
		if err == nil {
			t.Fatal("以 ! 开头的未加引号的值应该直接让整份配置解析失败")
		}
	})

	t.Run("unquoted value with a space before # gets silently truncated", func(t *testing.T) {
		got, err := loadYAMLValue(t, "db:\n  password: 12345 #not-a-comment\n", "db.password")
		if err != nil {
			t.Fatalf("LoadFile() error = %v", err)
		}
		// 未加引号时 # 会被当成注释，密码会被静默截断。
		if got == "12345 #not-a-comment" {
			t.Fatal("不应该：说明 YAML 解析器这次没有把 # 当注释处理，行为和预期不一致，需要重新评估这条测试")
		}
	})

	t.Run("double-quoted value preserves special characters exactly", func(t *testing.T) {
		got, err := loadYAMLValue(t, `db:
  password: "12345,./!@#$"
`, "db.password")
		if err != nil {
			t.Fatalf("LoadFile() error = %v", err)
		}
		if got != "12345,./!@#$" {
			t.Fatalf("password = %q, want %q（加了双引号还原本不应该走样）", got, "12345,./!@#$")
		}
	})

	t.Run("double-quoted value with a literal backslash and quote round-trips with escaping", func(t *testing.T) {
		// 双引号内的 \ 和 " 需要按 YAML 规则转义。
		got, err := loadYAMLValue(t, `db:
  password: "back\\slash and \"quotes\""
`, "db.password")
		if err != nil {
			t.Fatalf("LoadFile() error = %v", err)
		}
		want := `back\slash and "quotes"`
		if got != want {
			t.Fatalf("password = %q, want %q", got, want)
		}
	})
}
