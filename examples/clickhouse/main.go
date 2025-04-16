package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/boloc/go-frame-server/pkg/frame"
	"github.com/boloc/go-frame-server/pkg/frame/components"
	"github.com/boloc/go-frame-server/pkg/frame/config"
)

func main() {

	// 创建框架实例
	f := frame.New(
		frame.WithShutdownTimeout(30 * time.Second), // 设置30秒关闭超时
	)
	// 注册配置组件
	conf := config.MustLoad("frame-server", "./config")
	// 注册ClickHouse组件
	// 配置样例 - 如果未配置，请先在config文件中添加相应配置
	// clickhouse:
	//   default:
	//     addr: "localhost:9000"
	//     database: "default"
	//     username: "default"
	//     password: ""
	//     max_open_conns: 10
	//     max_idle_conns: 5
	//     conn_max_lifetime: "1h"
	//     dial_timeout: "10s"
	//     debug: true
	clickhouseComponent := components.NewClickHouseComponent(
		"shortlink",
		true,
		components.WithClickHouseAddress([]string{conf.GetString("clickhouse.default.addr")}),                        // 设置ClickHouse地址
		components.WithClickHouseDatabase(conf.GetString("clickhouse.default.database")),                             // 设置数据库名
		components.WithClickHouseUsername(conf.GetString("clickhouse.default.username")),                             // 设置用户名
		components.WithClickHousePassword(conf.GetString("clickhouse.default.password")),                             // 设置密码
		components.WithClickHouseMaxOpenConns(conf.GetInt("clickhouse.default.max_open_conns")),                      // 设置最大连接数
		components.WithClickHouseMaxIdleConns(conf.GetInt("clickhouse.default.max_idle_conns")),                      // 设置最大空闲连接数
		components.WithClickHouseConnMaxLifetime(conf.GetStringTimeDuration("clickhouse.default.conn_max_lifetime")), // 设置连接最大生命周期
		components.WithClickHouseDialTimeout(conf.GetStringTimeDuration("clickhouse.default.dial_timeout")),          // 设置连接超时时间
		components.WithClickHouseReadTimeout(conf.GetStringTimeDuration("clickhouse.default.read_timeout")),          // 设置读取超时时间
		components.WithClickHouseCompression(clickhouse.CompressionLZ4),                                              // 设置压缩方式
		components.WithClickHouseDebug(conf.GetBool("clickhouse.default.debug")),                                     // 设置调试
		components.WithClickHouseProtocol(conf.GetString("clickhouse.default.protocol")),                             // 设置协议
	)

	f.RegisterComponent(clickhouseComponent)

	// 注册启动后的操作
	f.AfterStart(func(ctx context.Context) error {
		// 执行查询
		// results := ClickHouseSelect(ctx)

		// 执行写入
		ClickHouseInsert(ctx)
		// ClickHouseQueryCount(ctx)
		ClickHouseQueryOne(ctx)

		// util.PrintFormat(results)
		// fmt.Println("results", results)
		return nil
	})

	// 运行框架
	if err := f.Run(); err != nil {
		fmt.Println("Framework error", err)
		os.Exit(1)
	}
}

// ShortlinkRecord 定义短链记录结构
type ShortlinkRecord struct {
	Cookie           string    `ch:"cookie" json:"cookie"`
	RequestTime      time.Time `ch:"request_time" json:"request_time"`
	IP               string    `ch:"ip" json:"ip"`
	Language         string    `ch:"language" json:"language"`
	Referer          string    `ch:"referer" json:"referer"`
	UserAgent        string    `ch:"user_agent" json:"user_agent"`
	UABrowser        string    `ch:"ua_browser" json:"ua_browser"`
	UABrowserVersion string    `ch:"ua_browser_version" json:"ua_browser_version"`
	UAOS             string    `ch:"ua_os" json:"ua_os"`
	UAOSVersion      string    `ch:"ua_os_version" json:"ua_os_version"`
	UADevice         string    `ch:"ua_device" json:"ua_device"`
	UADeviceBrand    string    `ch:"ua_device_brand" json:"ua_device_brand"`
	UADeviceModel    string    `ch:"ua_device_model" json:"ua_device_model"`
	UAApp            string    `ch:"ua_app" json:"ua_app"`
	UAAppVersion     string    `ch:"ua_app_version" json:"ua_app_version"`
	ShortDomain      string    `ch:"short_domain" json:"short_domain"`
	ShortCode        string    `ch:"short_code" json:"short_code"`
	RedirectURL      string    `ch:"redirect_url" json:"redirect_url"`
	RedirectDomain   string    `ch:"redirect_domain" json:"redirect_domain"`
	Country          string    `ch:"country" json:"country"`
	Region           string    `ch:"region" json:"region"`
	City             string    `ch:"city" json:"city"`
	ISP              string    `ch:"isp" json:"isp"`
	IsMobile         uint8     `ch:"is_mobile" json:"is_mobile"`
	ResponseTimeMS   uint32    `ch:"response_time_ms" json:"response_time_ms"`
	InsertTime       time.Time `ch:"insert_time" json:"insert_time"`
	UpdateTime       time.Time `ch:"update_time" json:"update_time"`
}

func ClickHouseSelect(ctx context.Context) []ShortlinkRecord {
	var results []ShortlinkRecord

	ch := frame.DefaultClickHouse()
	query := `
		SELECT
			cookie,
			request_time,
			ip,
			language,
			referer,
			user_agent,
			ua_browser,
			ua_browser_version,
			ua_os,
			ua_os_version,
			ua_device,
			ua_device_brand,
			ua_device_model,
			ua_app,
			ua_app_version,
			short_domain,
			short_code,
			redirect_url,
			redirect_domain,
			country,
			region,
			city,
			isp,
			is_mobile,
			response_time_ms,
			insert_time,
			update_time
		FROM shortlink_ods.shortlink_request_log
		WHERE request_time >= ?
		ORDER BY request_time DESC
		LIMIT ?
	`

	// 打印最终要执行的SQL
	requestTime := time.Date(2025, 10, 18, 0, 0, 0, 0, time.UTC)
	fmt.Println("打印时间", requestTime)

	args := []any{requestTime, 3}
	if err := ch.Select(context.Background(), &results, query, args...); err != nil {
		fmt.Printf("占位符方式查询失败: %v\n", err)
		return results // 返回空切片
	}

	return results
}

func ClickHouseInsert(ctx context.Context) {
	// 获取默认ClickHouse连接
	ch := frame.DefaultClickHouse()

	// 统计成功插入的记录数
	successCount := 0

	// 方式一：使用 Exec 方法和命名参数插入单条数据
	for i := 0; i < 5; i++ {
		// 为每条记录设置间隔1秒的时间，确保request_time不重复
		// requestTime := time.Now().Add(time.Duration(i) * time.Second)
		requestTime := time.Now()

		record := ShortlinkRecord{
			Cookie:           fmt.Sprintf("cookie_%d", i),
			RequestTime:      requestTime,
			IP:               fmt.Sprintf("192.168.1.%d", i),
			Language:         "zh-CN",
			Referer:          "https://example.com",
			UserAgent:        "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/100.0.4896.60 Safari/537.36",
			UABrowser:        "Chrome",
			UABrowserVersion: "100.0.4896.60",
			UAOS:             "Windows",
			UAOSVersion:      "10",
			UADevice:         "Desktop",
			UADeviceBrand:    "Unknown",
			UADeviceModel:    "PC",
			UAApp:            "",
			UAAppVersion:     "",
			ShortDomain:      "short.example.com",
			// ShortCode:        fmt.Sprintf("code%d", i),
			ShortCode:      "test_code",
			RedirectURL:    fmt.Sprintf("https://example.com/page%d", i),
			RedirectDomain: "example.com",
			Country:        "China",
			Region:         "Beijing",
			City:           "Beijing",
			ISP:            "China Telecom",
			IsMobile:       0,
			ResponseTimeMS: 15,
		}

		// 打印每条要插入的记录
		fmt.Printf("正在插入第 %d 条数据，request_time: %v\n", i+1, requestTime)

		err := ch.Exec(ctx, `
			INSERT INTO shortlink_ods.shortlink_request_log
			(cookie, request_time, ip, language, referer, user_agent, ua_browser, ua_browser_version,
			ua_os, ua_os_version, ua_device, ua_device_brand, ua_device_model, ua_app, ua_app_version,
			short_domain, short_code, redirect_url, redirect_domain, country, region, city, isp, is_mobile,
			response_time_ms)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ? )
		`,
			record.Cookie, record.RequestTime, record.IP, record.Language, record.Referer,
			record.UserAgent, record.UABrowser, record.UABrowserVersion, record.UAOS, record.UAOSVersion,
			record.UADevice, record.UADeviceBrand, record.UADeviceModel, record.UAApp, record.UAAppVersion,
			record.ShortDomain, record.ShortCode, record.RedirectURL, record.RedirectDomain, record.Country,
			record.Region, record.City, record.ISP, record.IsMobile, record.ResponseTimeMS)

		if err != nil {
			fmt.Printf("插入第 %d 条数据失败: %v\n", i+1, err)
			// 继续执行，不要return
		} else {
			successCount++
			fmt.Printf("第 %d 条数据插入成功\n", i+1)
		}
	}

	// // 方式二：使用批量插入
	// batch, err := ch.PrepareBatch(ctx, `
	// 	INSERT INTO shortlink_ods.shortlink_request_log
	// 	(cookie, request_time, ip, language, referer, user_agent, ua_browser, ua_browser_version,
	// 	ua_os, ua_os_version, ua_device, ua_device_brand, ua_device_model, ua_app, ua_app_version,
	// 	short_domain, short_code, redirect_url, redirect_domain, country, region, city, isp, is_mobile,
	// 	response_time_ms, insert_time, update_time)
	// `)
	// if err != nil {
	// 	fmt.Printf("准备批量插入失败: %v\n", err)
	// 	return
	// }

	// // 批量添加数据
	// batchSuccessCount := 0
	// for i := 5; i < 10; i++ {
	// 	requestTime := time.Now().Add(time.Duration(i) * time.Second)
	// 	now := time.Now()

	// 	err := batch.Append(
	// 		fmt.Sprintf("cookie_batch_%d", i), // cookie
	// 		requestTime,                       // request_time
	// 		fmt.Sprintf("192.168.1.%d", i),    // ip
	// 		"zh-CN",                           // language
	// 		"https://referrer.example.com",    // referer
	// 		"Mozilla/5.0 (iPhone; CPU iPhone OS 15_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148", // user_agent
	// 		"Safari",                 // ua_browser
	// 		"15.0",                   // ua_browser_version
	// 		"iOS",                    // ua_os
	// 		"15.0",                   // ua_os_version
	// 		"Mobile",                 // ua_device
	// 		"Apple",                  // ua_device_brand
	// 		"iPhone",                 // ua_device_model
	// 		"",                       // ua_app
	// 		"",                       // ua_app_version
	// 		"short.example.com",      // short_domain
	// 		fmt.Sprintf("code%d", i), // short_code
	// 		fmt.Sprintf("https://example.com/page%d", i), // redirect_url
	// 		"example.com",   // redirect_domain
	// 		"United States", // country
	// 		"California",    // region
	// 		"San Francisco", // city
	// 		"Comcast",       // isp
	// 		uint8(1),        // is_mobile
	// 		uint32(20),      // response_time_ms
	// 		now,             // insert_time
	// 		now,             // update_time
	// 	)
	// 	if err != nil {
	// 		fmt.Printf("添加批量数据失败: %v\n", err)
	// 		continue
	// 	}
	// 	batchSuccessCount++
	// }

	// // 发送批量数据
	// if err := batch.Send(); err != nil {
	// 	fmt.Printf("发送批量数据失败: %v\n", err)
	// } else {
	// 	successCount += batchSuccessCount
	// 	fmt.Printf("批量插入成功，添加了 %d 条记录\n", batchSuccessCount)
	// }

	fmt.Printf("数据插入完成，总共成功插入 %d 条记录\n", successCount)
}

// COUNT查询
func ClickHouseQueryCount(ctx context.Context) {
	ch := frame.DefaultClickHouse()
	// 一小时前
	oneHourAgo := time.Now().Add(-1 * time.Hour)
	query := "SELECT count(*) FROM shortlink_ods.shortlink_request_log WHERE request_time > ?"

	// 使用QueryRow和Scan获取单个值
	row := ch.QueryRow(context.Background(), query, oneHourAgo)

	var count uint64
	if err := row.Scan(&count); err != nil {
		fmt.Printf("计数查询失败: %v\n", err)
	} else {
		fmt.Printf("一小时前请求数: %d\n", count)
	}
}

// 查询一条记录
func ClickHouseQueryOne(ctx context.Context) {
	ch := frame.DefaultClickHouse()
	query := "SELECT * FROM shortlink_ods.shortlink_request_log WHERE cookie = ?"
	cookie := "cookie_1"
	row := ch.QueryRow(context.Background(), query, cookie)

	var result ShortlinkRecord
	if err := row.ScanStruct(&result); err != nil { // 使用ScanStruct将结果映射至结构体中
		// 没有记录
		if err == sql.ErrNoRows {
			fmt.Println("没有记录")
		} else {
			fmt.Printf("查询失败: %v\n", err)
		}
	} else {
		fmt.Printf("查询结果: %v\n", result)
	}
}
