# Go Frame Server

一个轻量级 Go 应用框架：统一管理组件生命周期（启动顺序、优雅关闭、SIGHUP 热重启），
提供常见基础设施（MySQL/Redis/ClickHouse/对象存储）和常见能力（幂等、限流、定时任务、
双层刷新缓存、健康检查、统一错误/响应、统一失败通知）的开箱即用实现。

`cmd/example` + `internal/example` 是一个完整的可运行示例应用，本文档提到的每一段代码都
能在那里找到对应的真实用法。

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
    "github.com/boloc/go-frame-server/pkg/frame/config"
)

func main() {
    // 配置路径优先级：-c 命令行参数 > CONFIG_FILE 环境变量 > 这里的默认值
    cfgPath := config.Resolve("./config/frame-server.yml")
    conf, err := config.LoadFile(cfgPath)
    if err != nil {
        panic(err)
    }

    f := frame.New(frame.WithShutdownTimeout(30 * time.Second))

    // 在这里用 conf 创建并注册需要的组件（MySQL/Redis/Gin/...），见下面"组件与能力一览"。

    if err := f.Run(); err != nil {
        panic(err)
    }
}
```

`frame.Frame` 本身只负责组件的启动顺序、优雅关闭、SIGHUP 热重启这几件事，**不持有任何业务
配置**——配置的加载、组件的创建都由业务代码自己决定，这样才能在单测里创建多个互不影响的
`Frame` 实例，也不会把配置加载的方式锁死在框架里。

### 3. 推荐结构：把组件初始化收进一个 `bootstrap` 包

完整参考 `cmd/example/bootstrap/`：

```bash
cmd/example/
├── main-demo.go          # 入口：加载配置 → New Frame → bootstrap.Setup → Run
├── bootstrap/
│   ├── bootstrap.go       # Setup 函数：整个应用"装配了哪些子系统"的目录索引，一个子系统一行
│   ├── timezone.go        # 进程时区
│   ├── logger.go          # 日志
│   ├── mysql.go            # MySQL（默认实例 + 命名实例）
│   ├── migration.go       # 数据库自动迁移（可选，默认关闭）
│   ├── redis.go           # Redis
│   ├── clickhouse.go      # ClickHouse
│   ├── cron.go             # 定时任务调度器
│   ├── refreshcache.go    # 双层刷新只读缓存实例
│   ├── storage.go         # 对象存储（可选能力）
│   ├── monitor.go         # Prometheus 指标采集
│   └── gin.go              # HTTP 服务
└── route/
    ├── route.go            # 路由入口
    └── *_route.go          # 按业务域拆分的路由文件
```

```go
// bootstrap.Setup 是整个应用"装配了哪些子系统"的目录索引：
func Setup(f *frame.Frame, conf *config.ConfigComponent) {
   SetupTimezone(conf)
   SetupLogger(f, conf)
    mysqlComponent := SetupMySQL(f, conf)
    // ... 其它命名 MySQL 实例、Redis、ClickHouse、定时任务、缓存、指标 ...
    SetupStorage(conf)
    SetupGin(f, conf, metricsHandlers)
}
```

业务层（handler/logic/repository）的目录结构和分层约定见
[`docs/api-conventions.md`](docs/api-conventions.md)，这里不重复。

## 配置文件

配置是纯 YAML + viper，框架不规定具体 schema，完整可用的示例见
[`config/frame-server.yml.example`](config/frame-server.yml.example)（复制成
`config/frame-server.yml` 后按需修改，这个文件默认被 gitignore，不会提交真实密钥）。

```bash
# 三种指定配置文件路径的方式，优先级从高到低：
./app -c /path/to/config.yml       # 命令行参数
CONFIG_FILE=/path/to/config.yml ./app  # 环境变量
config.Resolve("./config/frame-server.yml")  # 代码里的默认值
```

## 组件与能力一览

### 基础设施组件（`pkg/frame/components`，通过 `pkg/frame` 顶层函数访问）

| 组件 | 说明 |
| --- | --- |
| MySQL | 主从分离、连接失败重试、支持同一进程内多个命名实例；`frame.DefaultDB()`/`frame.MasterDB(name)` 等 panic 版访问器用在启动阶段，`frame.TryDefaultDB()`/`frame.TryMasterDB(name)` 等 Try 版本用在运行时代码里 |
| Redis | 单机/集群/哨兵三种模式，统一用 `frame.GetRedisCmdable()`/`frame.TryGetRedisCmdable()` 访问，业务代码不需要关心底层是哪种模式 |
| ClickHouse | 原生驱动 + GORM 两种接入方式 |
| Gin | HTTP 服务，内置请求体大小限制、请求超时、TrustedProxies 安全默认值、SIGHUP 热重启安全（路由只注册一次） |

```go
db     := frame.DefaultDB()                 // 默认实例主库，未连接时 panic，用在启动阶段
db, ok := frame.TryDefaultSlaveDB()         // 默认实例从库，未连接时 (nil, false)，用在运行时
db     := frame.MasterDB("config_db")        // 命名实例
rdb    := frame.GetRedisCmdable()           // 通用 Redis 接口，自动适配单机/集群/哨兵
```

### 通用能力

| 能力 | 包路径 | 说明 |
| --- | --- | --- |
| 统一响应/绑定/校验 | `pkg/frame/webx` | `webx.Success`/`webx.Fail`/`webx.FailWithStatus` + `webx.Bind`/`BindQuery`/`BindJSON`/`BindURI`，绑定后自动跑 `validate:` tag 校验 |
| 统一错误类型 | `pkg/errs` | `*errs.Error`：显式错误码、HTTP 状态码映射、默认文案，`errs.Database`/`errs.NotFound` 等便捷构造函数 |
| 幂等 | `pkg/frame/idempotency` | 基于 Redis `SETNX` 的幂等中间件，可配置 `Required`/`FailOpen` |
| 限流 | `pkg/frame/ratelimit` | 基于 Redis + Lua 原子操作的固定窗口限流中间件 |
| 定时任务 | `pkg/frame/cron` | Component 化调度器，防并发重入、执行指标、优雅关闭；支持标准 cron 表达式和 `@every` |
| 双层刷新只读缓存 | `pkg/frame/refreshcache` | 数据源 → Redis → 内存两层定时刷新，Go 泛型类型安全 API，适合读多写少、允许有界延迟的数据 |
| 健康检查 | `pkg/frame/healthcheck` | `/health` 处理器，短 TTL 缓存结果，每个依赖单独超时，区分关键/非关键依赖 |
| 统一失败通知 | `pkg/alert` | 全局单 Hook，覆盖框架各处的失败事件（HTTP/`cron`/`idempotency`/`ratelimit`/组件启停），业务只需要 `alert.SetHook` 接一次 |
| 对象存储 | `pkg/frame/storage` | S3 兼容对象存储客户端（AWS S3/Cloudflare R2/MinIO/...），见该目录的 [README](pkg/frame/storage/README.md) |
| 进程时区 | `pkg/util.SetProcessTimezone` | 设置 `time.Local`；MySQL 连接的时区跟这个无关，默认固定 UTC，见下方"时区"一节 |
| 分页 | `pkg/frame/pagination` | GORM 分页 scope + 统一的分页请求/响应结构 |
| 参数校验 | `pkg/frame/validate` | `validate:` tag 校验，逐字段中文说明，`webx.Bind*` 内部自动调用 |
| 请求上下文 | `pkg/frame/reqctx` | 请求级数据（RequestID/ClientIP/UserAgent/请求体），配合 `pkg/frame/middleware.ContextMiddleware` |

### 中间件（`pkg/frame/middleware`）

| 中间件 | 说明 |
| --- | --- |
| `ContextMiddleware` | 生成/回显 `X-Request-Id`，捕获请求体供 `reqctx` 使用 |
| `BodyLimit` | 限制单次请求体大小 |
| `RequestTimeout` | 给请求 `ctx` 挂整体超时 |

## 分层与编码约定

handler 命名、`validate:` tag vs 业务规则校验层、DTO/Model 分层、repository 的 master/
slave 选择、跨命名实例编排该放哪一层……这些团队约定统一写在
[`docs/api-conventions.md`](docs/api-conventions.md)，新人接入项目应该先看这份文档，不是
先看框架源码。

## 时区

- `server.timezone` 配置项（默认 `UTC`）决定 `time.Now()`、日志时间戳等"没有显式指定时区
  的代码"用哪个时区，由 `bootstrap.SetupTimezone` 在启动最开始设置。
- **MySQL 连接的时区跟 `server.timezone` 无关**，默认固定 `UTC`（`database.<name>.master.loc`
  留空时的默认值），需要别的时区要在那个实例上显式配置——这是故意解耦的，存储层的时区
  约定不应该随业务展示时区的调整而跟着变。
- 把一个 `time.Time` 展示给用户，统一用 `util.FormatLocal(t)`（先转换到 `time.Local` 再
  格式化），不要直接对数据库读出来的值调 `.Format()`。

## 优雅关闭与热重启

```bash
kill -SIGTERM <pid>   # 或 Ctrl+C：优雅关闭——BeforeStop 钩子 → 按注册的反序 Stop 组件
kill -SIGHUP  <pid>   # 热重启——Stop 再 Start 所有组件，不退出进程，不重新执行 AfterStart/BeforeStop
```

`f.AfterStart(...)`/`f.BeforeStop(...)` 可以注册任意多个钩子，按注册顺序依次执行。

## 深入了解

- [`docs/api-conventions.md`](docs/api-conventions.md)：分层/命名/校验规范，团队协作时应该遵守的规则。
- [`docs/framework-writing-and-improvement.md`](docs/framework-writing-and-improvement.md)：架构决策的调研过程和权衡取舍，想知道"为什么这么设计"看这里。
- [`pkg/frame/storage/README.md`](pkg/frame/storage/README.md)：对象存储客户端的配置项/接入指南。
- `cmd/example` + `internal/example`：完整可运行的示例应用，覆盖本文档提到的绝大多数能力，直接 `go run` 起来对照着看最直观。

## License

MIT
