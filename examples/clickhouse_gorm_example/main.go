package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/boloc/go-frame-server/pkg/constant"
	"github.com/boloc/go-frame-server/pkg/frame"
	"github.com/boloc/go-frame-server/pkg/frame/components"
	"github.com/boloc/go-frame-server/pkg/frame/config"
	"github.com/boloc/go-frame-server/pkg/util"
	"gorm.io/gorm"
)

// ShortlinkRecord 短链记录数据结构
type ShortlinkRecord struct {
	ID             uint      `gorm:"primaryKey"`
	AccountCode    string    `gorm:"column:account_code;type:String" ch:"account_code" json:"account_code"`
	Cookie         string    `gorm:"column:cookie;type:String" ch:"cookie" json:"cookie"`
	RequestTime    time.Time `gorm:"column:request_time;type:DateTime" ch:"request_time" json:"request_time"`
	IP             string    `gorm:"column:ip;type:String" ch:"ip" json:"ip"`
	AcceptLanguage string    `gorm:"column:accept_language;type:String" ch:"accept_language" json:"accept_language"`
	LanguageCode   string    `gorm:"column:language_code;type:String" ch:"language_code" json:"language_code"`
	Referer        string    `gorm:"column:referer;type:String" ch:"referer" json:"referer"`
}

// TableName 设置默认表名
func (ShortlinkRecord) TableName() string {
	return "shortlink_request_log"
}

// ShortlinkRecordRepository 短链记录仓库
type ShortlinkRecordRepository struct {
	db *gorm.DB
}

// NewShortlinkRecordRepository 创建短链记录仓库
func NewShortlinkRecordRepository() *ShortlinkRecordRepository {
	return &ShortlinkRecordRepository{
		db: frame.DefaultClickHouseDB(),
	}
}

// RecordClick 记录点击 - 插入到本地表
func (r *ShortlinkRecordRepository) RecordClick(ctx context.Context, record *ShortlinkRecord) error {
	// 直接使用Table方法指定要操作的表名
	result := r.db.Table("shortlink_request_log_local").Create(record)
	return result.Error
}

func main() {
	// 创建框架实例
	f := frame.New(
		frame.WithShutdownTimeout(30 * time.Second), // 设置30秒关闭超时
	)

	// 注册配置组件 - 这里只是为了初始化配置系统
	conf := config.MustLoad("frame-server", "./config")

	// 创建ClickHouse GORM组件
	clickhouseDSN := util.BuildClickhouseDSN(map[string]any{
		"user":     conf.GetString("clickhouse.default.username"), // 用户名
		"password": conf.GetString("clickhouse.default.password"), // 密码
		"host":     conf.GetString("clickhouse.default.host"),     // 地址
		"port":     conf.GetString("clickhouse.default.port"),     // 端口
		"name":     conf.GetString("clickhouse.default.database"), // 数据库
		"loc":      "UTC",                                         // 时区
	})
	clickhouseGorm := components.NewClickHouseGORMComponent(
		"shortlink",
		&components.ClickHouseGORMConfig{
			DSN:             clickhouseDSN,
			MaxIdleConns:    conf.GetInt("clickhouse.default.max_idle_conns"),                   // 最大空闲连接数
			MaxOpenConns:    conf.GetInt("clickhouse.default.max_open_conns"),                   // 最大连接数
			ConnMaxLifetime: conf.GetStringTimeDuration("clickhouse.default.conn_max_lifetime"), // 连接最大生命周期
			LogLevel:        components.GormLogLevelForEnv(constant.EnvTest),                    // 日志等级                                                // 日志级别
		},
		true, // 设为默认实例
	)

	// 注册组件
	f.RegisterComponent(clickhouseGorm)

	// 注册启动后的操作
	f.AfterStart(func(ctx context.Context) error {
		// 创建仓库
		repo := NewShortlinkRecordRepository()

		// 创建记录
		record := &ShortlinkRecord{
			AccountCode:    "sitepower_o",
			Cookie:         "test_cookie",
			RequestTime:    time.Now(),
			IP:             "127.0.0.1",
			AcceptLanguage: "en-US",
			LanguageCode:   "en",
			Referer:        "http://example.com",
		}

		// 尝试插入记录 - 会插入到本地表
		fmt.Println("尝试插入记录到本地表 shortlink_request_log_local...")
		if err := repo.RecordClick(ctx, record); err != nil {
			fmt.Printf("插入记录失败: %v\n", err)
		} else {
			fmt.Println("插入记录成功!")
		}

		// 查询记录 - 从分布式表查询
		fmt.Println("\n尝试从分布式表 shortlink_request_log 查询记录...")
		var records []ShortlinkRecord
		result := frame.DefaultClickHouseDB().
			Where("account_code = ?", "sitepower_o").
			Limit(10).
			Find(&records)

		if result.Error != nil {
			fmt.Printf("查询记录失败: %v\n", result.Error)
		} else {
			fmt.Printf("查询到 %d 条记录\n", len(records))
			for i, r := range records {
				fmt.Printf("记录 %d: %s - %s\n", i+1, r.AccountCode, r.IP)
			}
		}

		return nil
	})

	// 运行框架
	if err := f.Run(); err != nil {
		fmt.Println("Framework error", err)
		os.Exit(1)
	}
}
