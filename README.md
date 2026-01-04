# Go Frame Server

一个轻量级的 Go 应用框架，提供配置管理、数据库连接、缓存、Web 服务等组件的统一管理。

## 特性

- **配置管理** - 支持 YAML 配置，多优先级加载（环境变量 > 命令行 > 默认值）
- **MySQL** - 主从分离、连接池、自动重连
- **Redis** - 单机/集群模式，统一接口
- **ClickHouse** - GORM 集成
- **Gin** - Web 服务集成
- **日志** - 基于 Zap，支持文件轮转
- **生命周期** - 启动/停止钩子，优雅关闭

## 快速开始

### 1. 安装

```bash
go get github.com/boloc/go-frame-server
```

### 2. 最小示例

```go
package main

import (
    "time"
    "github.com/boloc/go-frame-server/pkg/frame"
)

func main() {
    f := frame.New(
        frame.WithConfigFile("./config/app.yml"),
        frame.WithShutdownTimeout(30 * time.Second),
    )

    // 初始化组件...

    if err := f.Run(); err != nil {
        panic(err)
    }
}
```

### 3. 完整示例（推荐结构）

```js
cmd/client/
├── main.go
├── bootstrap/
│   ├── bootstrap.go    # 组件初始化入口
│   ├── logger.go       # 日志配置
│   ├── mysql.go        # MySQL 配置
│   ├── redis.go        # Redis 配置
│   ├── clickhouse.go   # ClickHouse 配置
│   └── gin.go          # Gin 配置
└── route/
    └── route.go        # 路由定义
```

**main.go**

```go
package main

import (
    "context"
    "fmt"
    "os"
    "time"

    "your-project/cmd/client/bootstrap"
    "github.com/boloc/go-frame-server/pkg/frame"
)

func main() {
    // 创建框架实例
    // 配置优先级：环境变量 CONFIG_FILE > -c 命令行参数 > 默认值
    f := frame.New(
        frame.WithConfigFile("./config/app.yml"),
        frame.WithShutdownTimeout(30 * time.Second),
    )

    // 初始化所有组件
    bootstrap.Setup(f)

    // 启动后钩子
    f.AfterStart(func(ctx context.Context) error {
        fmt.Println("服务已启动")
        return nil
    })

    // 停止前钩子
    f.BeforeStop(func(ctx context.Context) error {
        fmt.Println("服务即将停止")
        return nil
    })

    // 运行
    if err := f.Run(); err != nil {
        fmt.Println("启动失败:", err)
        os.Exit(1)
    }
}
```

## 配置文件

框架支持三种方式指定配置文件：

```bash
# 1. 环境变量（最高优先级）
export CONFIG_FILE=/path/to/config.yml
./app

# 2. 命令行参数
./app -c /path/to/config.yml

# 3. 代码默认值（最低优先级）
frame.WithConfigFile("./config/app.yml")
```

### 配置示例

```yaml
# config/app.yml

# 服务配置
server:
  port: 8080
  env: local  # local/test/production

# 日志配置
logs:
  log_level: debug      # debug/info/warn/error
  is_stdout: true       # 是否输出到标准输出
  is_file: true         # 是否输出到文件
  file_name: app.log    # 日志文件名（输出到 logs/ 目录）
  max_size: 100         # 单个文件最大尺寸 MB
  max_backups: 10       # 最多保留备份数
  max_age: 30           # 最多保留天数
  compress: true        # 是否压缩

# MySQL 配置
database:
  default_db:                           # 数据库实例名
    max_idle_conns: 15                  # 空闲连接池大小
    max_open_conns: 30                  # 最大连接数
    conn_max_lifetime: 30m              # 连接最大生命周期
    prefix: ""                          # 表前缀
    master:                             # 主库配置
      host: 127.0.0.1
      port: 3306
      user: root
      password: your-password
      name: your_database
      charset: utf8mb4
      collation: utf8mb4_unicode_ci
      parse_time: true
      loc: Asia/Shanghai
    # slaves:                           # 从库配置（可选）
    #   - host: 127.0.0.1
    #     port: 3307
    #     user: root
    #     password: your-password
    #     name: your_database
    #     charset: utf8mb4

# Redis 配置
redis:
  single:                               # 单机模式
    addr: 127.0.0.1:6379
    password: ""
    db: 0
    pool_size: 10                       # 连接池大小
    min_idle_conns: 10                  # 最小空闲连接数
    dial_timeout: 5s                    # 连接超时
    read_timeout: 5s                    # 读取超时
    write_timeout: 5s                   # 写入超时
    max_retries: 3                      # 最大重试次数
  cluster:                              # 集群模式
    password: ""
    pool_size: 10
    min_idle_conns: 10
    dial_timeout: 5s
    read_timeout: 5s
    write_timeout: 5s
    max_retries: 3
    route_randomly: true                # 是否随机路由
    min_retry_backoff: 100ms            # 最小重试间隔
    max_retry_backoff: 2s               # 最大重试间隔
    nodes:                              # 集群节点
      - 127.0.0.1:7001
      - 127.0.0.1:7002
      - 127.0.0.1:7003

# ClickHouse 配置
clickhouse:
  default_ch:                           # 实例名
    host: 127.0.0.1
    port: 9000
    database: default
    username: default
    password: ""
    max_idle_conns: 5                   # 最大空闲连接数
    max_open_conns: 10                  # 最大连接数
    conn_max_lifetime: 1h               # 连接最大生命周期
    log_level: local                    # 日志级别
```

## 组件使用

### MySQL

```go
import "github.com/boloc/go-frame-server/pkg/frame"

// 获取默认主库连接
db := frame.DefaultDB()

// 获取默认从库连接（自动轮询）
slaveDB := frame.DefaultSlaveDB()

// 获取指定实例
db := frame.MasterDB("another_db")
slaveDB := frame.SlaveDB("another_db")

// 使用示例
var users []User
db.Where("status = ?", 1).Find(&users)
```

### Redis

```go
import (
    "time"
    "github.com/boloc/go-frame-server/pkg/frame"
)

// 方式1：通用接口（推荐）- 自动适配单机/集群
rdb := frame.GetRedisCmdable()
rdb.Set(ctx, "key", "value", time.Hour)
val, _ := rdb.Get(ctx, "key").Result()

// 方式2：明确使用单机客户端
client := frame.GetRedis()

// 方式3：明确使用集群客户端
cluster := frame.GetRedisCluster()
```

**推荐使用 `GetRedisCmdable()`**，业务代码无需关心底层是单机还是集群，切换时无需修改代码。

### ClickHouse

```go
import "github.com/boloc/go-frame-server/pkg/frame"

// 获取 ClickHouse GORM 连接
db := frame.DefaultClickHouseDB()

// 查询示例
var results []YourModel
db.Table("your_table").Where("date = ?", today).Find(&results)
```

### 获取配置

```go
// 方式1：通过框架实例
conf := f.Config()
port := conf.GetString("server.port")

// 方式2：全局获取
import "github.com/boloc/go-frame-server/pkg/frame/config"

conf := config.GetConfig()
env := conf.GetString("server.env")

// 常用方法
conf.GetString("key")                    // 获取字符串
conf.GetInt("key")                       // 获取整数
conf.GetBool("key")                      // 获取布尔值
conf.GetStringTimeDuration("key")        // 获取时间（如 "5s", "1h"）
conf.GetStringSlice("key")               // 获取字符串数组
conf.GetStringMap("key")                 // 获取 map
```

## 生命周期

```go
f := frame.New(...)

// 启动后执行（可注册多个）
f.AfterStart(func(ctx context.Context) error {
    // 初始化缓存、预热数据等
    return nil
})

// 停止前执行（可注册多个）
f.BeforeStop(func(ctx context.Context) error {
    // 清理资源、保存状态等
    return nil
})

f.Run()
```

## 优雅关闭

框架自动处理 `SIGINT` 和 `SIGTERM` 信号：

1. 停止接收新请求
2. 执行 `BeforeStop` 钩子
3. 等待进行中的请求完成（最多等待 `ShutdownTimeout`）
4. 关闭所有组件（按注册的反序）

```bash
# 发送停止信号
kill -SIGTERM <pid>
# 或 Ctrl+C
```

## 目录结构建议

```js
your-project/
├── cmd/
│   └── client/
│       ├── main.go
│       ├── bootstrap/
│       │   ├── bootstrap.go
│       │   ├── logger.go
│       │   ├── mysql.go
│       │   ├── redis.go
│       │   ├── clickhouse.go
│       │   └── gin.go
│       └── route/
│           └── route.go
├── config/
│   ├── app.yml
│   └── app.yml.example
├── internal/
│   ├── handler/      # HTTP 处理器
│   ├── service/      # 业务逻辑
│   ├── repository/   # 数据访问
│   └── model/        # 数据模型
├── pkg/
│   └── util/         # 工具函数
├── logs/
├── go.mod
└── README.md
```

## License

MIT
