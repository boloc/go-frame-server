# Go Frame Server

一个轻量级 Go 应用框架（Go 1.27）：统一管理组件生命周期（启动顺序、优雅关闭、SIGHUP 热重启），
提供常见基础设施（MySQL/Redis/ClickHouse/对象存储）和常见能力（幂等、限流、定时任务、
双层刷新缓存、健康检查、统一错误/响应、统一失败通知）的开箱即用实现。RequestID、对象存储
默认对象键等使用标准库 `uuid`。

`cmd/example` + `internal/example` 是一个完整的可运行示例应用，本文档提到的每一段代码都
能在那里找到对应的真实用法。

## 文档地图

每份文档只覆盖一类问题。路径和配置 key 以代码为准，文档不另造别名。


| 文档                                                                     | 角色        | 你在这里找什么                 | 权威出处                                      |
| ---------------------------------------------------------------------- | --------- | ----------------------- | ----------------------------------------- |
| 本文                                                                     | 总览        | 框架是什么、装哪些组件、生产检查清单      | `pkg/frame`、`cmd/example`                 |
| `[docs/dev-commands.md](docs/dev-commands.md)`                         | How-to    | 启动、清端口、探活、复制即用 curl | `config/frame-server.yml` 的 `server.port` |
| `[docs/api-conventions.md](docs/api-conventions.md)`                   | 团队规范      | 命名、分层、校验、幂等/限流/访问器      | `cmd/example` + `internal/example`        |
| `[docs/example-apifox.openapi.json](docs/example-apifox.openapi.json)` | API 参考    | 导入 Apifox：路径、请求头、字段、错误码 | `cmd/example/route` + dto/handler         |
| `[config/frame-server.yml.example](config/frame-server.yml.example)`   | 配置参考      | 键名和示例默认值（端口示例是 `10005`） | 复制为 `config/frame-server.yml`             |
| `[pkg/frame/storage/README.md](pkg/frame/storage/README.md)`           | 存储 How-to | 对象存储接入                  | `pkg/frame/storage`                       |




## 快速开始



### 1. 安装

```bash
go get github.com/boloc/go-frame-server/v2
```



### 2. 最小示例

```go
package main

import (
    "time"

    "github.com/boloc/go-frame-server/v2/pkg/frame"
    "github.com/boloc/go-frame-server/v2/pkg/frame/config"
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
`Frame` 实例，也不会把配置加载的方式锁死在框架里。完整装配见 `cmd/example/main.go`
（`config.Resolve` → `config.LoadFile` → `bootstrap.Setup` 里的 `MustStrictUnmarshalKey`）。

### 3. 推荐结构：把组件初始化收进一个 `bootstrap` 包

完整参考 `cmd/example/bootstrap/`：

```bash
cmd/example/
├── main.go               # 入口：加载配置 → New Frame → bootstrap.Setup → Run
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
    ├── route.go            # 路由入口（/livez /readyz /metrics）
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
`[docs/api-conventions.md](docs/api-conventions.md)`，这里不重复。

## 配置文件

配置是纯 YAML + viper，框架不规定具体 schema，完整可用的示例见
`[config/frame-server.yml.example](config/frame-server.yml.example)`（复制成
`config/frame-server.yml` 后按需修改，这个文件默认被 gitignore，不会提交真实密钥）。

```bash
# 三种指定配置文件路径的方式，优先级从高到低：
./app -c /path/to/config.yml       # 命令行参数
CONFIG_FILE=/path/to/config.yml ./app  # 环境变量
config.Resolve("./config/frame-server.yml")  # 代码里的默认值
```

配置段用 `MustStrictUnmarshalKey` 解析：未知字段或 `validate:` 必填缺失会让启动失败
（见 `cmd/example/bootstrap/*.go`）。

## 组件与能力一览



### 基础设施组件（`pkg/frame/components`，通过 `pkg/frame` 顶层函数访问）


| 组件         | 说明                                                                                                                                                                                                                               | 示例位置                                                                                                                                                                             |
| ---------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| MySQL      | 主从分离、连接失败重试、同一进程多个命名实例。表名前缀来自 `database.<name>.prefix`；`SystemConfig`（config_db）和 `OperationLog`（log_db）不实现 `TableName()`，走 GORM 默认命名策略。panic 版访问器用在启动期和「服务期一定连着」的 repository；可选依赖或探测用 `Try*`，见 [访问器约定](docs/api-conventions.md) | `cmd/example/bootstrap/mysql.go`；从库读/主库写：`internal/example/repository/product_repository.go`；命名实例：`system_config_repository.go`（config_db）、`operation_log_repository.go`（log_db） |
| Redis      | 单机/集群/哨兵，业务统一 `frame.GetRedisCmdable()` / `TryGetRedisCmdable()`。go-redis 内部日志经 `pkg/logger`（Warn，`component=redis`）落地                                                                                                           | `cmd/example/bootstrap/redis.go`；被幂等、限流、`cron.Task.Exclusive`、refreshcache 使用                                                                                                    |
| ClickHouse | 原生驱动 + GORM。示例走 GORM：`components.TryDefaultClickHouseDB`                                                                                                                                                                         | `cmd/example/bootstrap/clickhouse.go`；读写：`POST /test/clickhouse/events`                                                                                                          |
| Gin        | HTTP 服务，内置 MaxBodyBytes、RequestTimeout、TrustedProxies、路由只注册一次                                                                                                                                                                    | `cmd/example/bootstrap/gin.go`                                                                                                                                                   |


```go
db     := frame.DefaultDB()                 // 默认实例主库，未连接时 panic
db, ok := frame.TryDefaultSlaveDB()         // 默认实例从库，未连接时 (nil, false)
db     := frame.MasterDB("config_db")        // 命名实例
rdb    := frame.GetRedisCmdable()           // 通用 Redis 接口，自动适配单机/集群/哨兵
```



### 通用能力


| 能力         | 包路径                                     | 说明                                                                                              | 示例位置                                                                                                    |
| ---------- | --------------------------------------- | ----------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- |
| 统一响应/绑定/校验 | `pkg/frame/webx`                        | `Success` / `Fail` / `FailWithStatus` + `Bind*`；`SetOnFail`、`IncludeCallerInResponse`、`BizCode` | `/test/success`、`/test/not-found`、`/test/not-found-with-status`；钩子与 Caller 开关：`cmd/example/main.go`     |
| 统一错误类型     | `pkg/errs`                              | `*errs.Error`；`RegisterHTTPStatus` / `RegisterMessage`                                          | `/test/business-error` + `internal/example/bizerr`                                                      |
| 幂等         | `pkg/frame/idempotency`                 | Redis `SETNX`；`Required` / `FailOpen` / `WithScopeFunc`                                         | `POST /api/orders`（`cmd/example/route/order_route.go`）                                                  |
| 限流         | `pkg/frame/ratelimit`                   | Redis + Lua 固定窗口；必须显式 `WithLimit`/`WithWindow`。中间件可挂在组上，计数按路由 `FullPath` × ClientIP，不是整组共用一个桶   | `/api/products` 组（`cmd/example/route/product_route.go`）                                                 |
| 定时任务       | `pkg/frame/cron`                        | 防并发重入、执行指标、优雅关闭；`Exclusive` 走 Redis 锁                                                           | `cmd/example/cron/cron.go`；`GET /api/cron/tasks`                                                        |
| 双层刷新只读缓存   | `pkg/frame/refreshcache`                | 数据源 → Redis → 内存；`Get` 内存未就绪才回落 Loader；`Jitter` 错开刷新                                            | `GET /api/products/summary`、详情页公告 `GET /api/products/:id`                                               |
| Redis key 命名空间 | `pkg/frame/rediskey`                  | 命名空间由应用注入（取 `server.name`），框架不给默认值；四类能力自动拼，业务 key 用 `App` 显式拼                                     | `cmd/example/bootstrap/redis.go`；`GET /test/redis-key`                                                   |
| 健康检查       | `pkg/frame/healthcheck`                 | `/livez` 存活；`/readyz`（及兼容别名 `/health`）就绪                                                        | `cmd/example/route/route.go`                                                                            |
| 统一失败通知     | `pkg/alert`                             | 全局单 Hook；`Dropped()` 是打满 64 并发后的丢弃计数                                                            | `cmd/example/main.go` 的 `alert.SetHook`；`GET /test/alert-dropped`                                       |
| 对象存储       | `pkg/frame/storage`                     | S3 兼容客户端                                                                                        | `/test/storage/*`（`cmd/example/route/storage_route.go`）                                                 |
| 进程时区       | `pkg/util.SetProcessTimezone`           | 设置 `time.Local`；展示时间用 `util.FormatLocal`                                                        | `cmd/example/bootstrap/timezone.go`；`GET /test/timezone`                                                |
| 分页         | `pkg/frame/pagination`                  | GORM scope + 统一分页请求/响应                                                                          | `GET /api/products/list`                                                                                |
| 参数校验       | `pkg/frame/validate`                    | `validate:` tag；可选 `Validatable`                                                                | tag：`GET /test/validation-error`；`Validatable`：`POST /test/validatable`；validation 层：`POST /api/orders` |
| 请求上下文      | `pkg/frame/reqctx`                      | RequestID / ClientIP / RequestBody / CustomData                                                 | `GET`/`POST /test/reqctx`                                                                               |
| 出站 HTTP    | `pkg/util.GetClient` / `GetNamedClient` | resty 客户端，默认重试幂等方法                                                                              | `/test/http-client/*`                                                                                   |
| 枚举下拉       | `pkg/util/options`                      | 一次 `Put` 同时拿列表和查找表                                                                              | `GET /api/products/options`（`internal/example/enum/product.go`）                                         |
| 集合辅助       | `pkg/util/maps`                         | `MapBuilder` / `SliceToMap` / `GetOrDefault` 等                                                  | `GET /test/maps`                                                                                        |
| 日志         | `pkg/logger`                            | 字段化 `logger.Info(..., zap.String(...))`；`WithLoggerStdoutJSON`                                  | `cmd/example/bootstrap/logger.go`；`GET /test/logger`                                                    |
| 进程指标       | `pkg/monitor`                           | 内存/goroutine 采集 + `HTTPMetrics` + `/metrics` Basic Auth                                         | `cmd/example/bootstrap/monitor.go`                                                                      |




### 中间件（`pkg/frame/middleware`）


| 中间件                      | 说明                                                            | 示例位置                                                                  |
| ------------------------ | ------------------------------------------------------------- | --------------------------------------------------------------------- |
| `ContextMiddleware`      | 生成/回显 `X-Request-Id`，捕获请求体供 `reqctx` 使用                       | `cmd/example/bootstrap/gin.go`；字段演示 `/test/reqctx`                    |
| `AccessLog`              | 结构化访问日志（含 `biz_code`、`request_id`、耗时）；默认跳过探针和 `/metrics`      | `cmd/example/bootstrap/gin.go`（`WithSlowThreshold(1s)`）               |
| `MaxBodyBytes`           | 限制单次请求体大小（GinComponent 默认 4MB）                                | 由 `NewGinComponent` 内置挂载；演示 `POST /test/body-limit`                   |
| `RequestTimeout`         | 给请求 `ctx` 挂整体超时，不写响应                                          | 由 `NewGinComponent` 按 `server.request_timeout` 挂载；演示 `GET /test/slow` |
| `monitor.HTTPMetrics`    | `http_requests_total` / duration / in-flight，读 `webx.BizCode` | `cmd/example/bootstrap/gin.go`                                        |
| `idempotency.Middleware` | 按路由/分组挂载，不要做成全局中间件                                            | `POST /api/orders`                                                    |
| `ratelimit.Middleware`   | 按路由/分组挂载；默认 key = `FullPath` + ClientIP                       | `/api/products` 组；每条路径各自 10 次/分钟                                      |




## 分层与编码约定

handler 命名、`validate:` tag vs 业务规则校验层、DTO/Model 分层、repository 的 master/
slave 选择、panic / Try 访问器、跨命名实例编排该放哪一层……这些团队约定统一写在
`[docs/api-conventions.md](docs/api-conventions.md)`，新人接入项目应该先看这份文档，不是
先看框架源码。

## 时区

- `server.timezone` 配置项（默认 `UTC`）决定 `time.Now()`、日志时间戳等"没有显式指定时区
的代码"用哪个时区，由 `bootstrap.SetupTimezone` 在启动最开始设置。
- **MySQL 连接的时区跟** `server.timezone` **无关**，默认固定 `UTC`（`database.<name>.master.loc`
留空时的默认值），需要别的时区要在那个实例上显式配置——这是故意解耦的，存储层的时区
约定不应该随业务展示时区的调整而跟着变。DSN 建连超时（`timeout`）留空默认 **5s**。
会话 `time_zone` 与连接 `loc` 对齐（UTC 时 DSN 带 `time_zone='+00:00'`）：`loc` 只解释
DATETIME 裸字符串，`time_zone` 决定服务端 `NOW()` / 列默认值；两边不一致会让库内默认
时间被 Go 按错误时区读出。
- 把一个 `time.Time` 展示给用户，统一用 `util.FormatLocal(t)`（先转换到 `time.Local` 再
格式化），不要直接对数据库读出来的值调 `.Format()`。对照见 `GET /test/timezone`。



## 健康检查：`/livez` 与 `/readyz`


| 路径            | handler                    | 接到                                                                                                |
| ------------- | -------------------------- | ------------------------------------------------------------------------------------------------- |
| `GET /livez`  | `healthcheck.Liveness()`   | k8s **livenessProbe**。进程活着就 200，不检查依赖。                                                            |
| `GET /readyz` | `healthcheck.Handler(...)` | k8s **readinessProbe**。关键依赖（示例里是 default_db / config_db / redis）失败返回 503，摘流量；log_db 为非关键，失败不影响就绪。 |
| `GET /health` | 与 `/readyz` 同一 handler     | 兼容别名，不要接到 liveness。                                                                               |


不要把 `/readyz` 接到 liveness：Redis 一挂 k8s 会把所有 Pod 循环重启。装配见
`cmd/example/route/route.go`。

## 幂等键的作用域

默认 Redis key = `<server.name>:idemp:` + 路由 `FullPath` + `:` + 客户端 `Idempotency-Key`。
最外层是 `server.name` 注入的命名空间，框架四类 Redis key（幂等、限流、定时任务锁、刷新缓存）
都从它拼出来，完整组成见 `pkg/frame/rediskey` 的包文档。

- 客户端用全局唯一 UUID，或接口没有用户概念时，**不需要** scope。
- 键可能跨用户重复（大家都传 `"1"`），或要防止别人猜键重放响应时，用
`idempotency.WithScopeFunc` 把用户标识拼进 key。

示例：`POST /api/orders` 用 `X-Demo-Token` 当演示用户，同一 token + 同一
`Idempotency-Key` 重放同一响应；换 token 同 key 各下一单。见
`cmd/example/route/order_route.go`。**禁止**用 `X-Request-Id` 当幂等键。

## 优雅关闭与热重启

```bash
kill -SIGTERM <pid>   # 或 Ctrl+C：优雅关闭——BeforeStop 钩子 → 按注册的反序 Stop 组件
kill -SIGHUP  <pid>   # 仅当 WithRestartSignal(true) 时：Stop 再 Start 所有组件，不退出进程
```

SIGHUP 热重启**默认关闭**。需要时在 `frame.New` 里加 `frame.WithRestartSignal(true)`。

代价：SIGHUP **不会重新读配置文件**；重启期间 HTTP 端口会短暂关闭；**不会**重跑
`AfterStart` / `BeforeStop`。多副本部署请走滚动重启，不要靠 SIGHUP。

`f.AfterStart(...)` / `f.BeforeStop(...)` 可以注册任意多个钩子，按注册顺序依次执行。
示例里 AfterStart 打印已注册路由数并校验 MySQL/Redis 已就绪，BeforeStop 发一条
「服务下线」的 `alert.Notify`，见 `cmd/example/main.go`。

## 生产部署检查清单

框架的设计目标是"配置错了就启动不起来，启动起来了就别在运行期出意外"，下面这些是启动
时**会**被拦住的，以及需要你在部署时自己确认的：

- 启动即失败：配置文件读不到、配置段严格解析失败（未知字段或必填缺失）、
`logs.log_level` 不是 debug/info/warn/error、MySQL 的 host/port/name/user 缺失或
`loc`/`timeout` 非法、主库/Redis/ClickHouse 连不上（按 `ConnectRetryAttempts` 重试后
仍失败）、cron 表达式非法或任务名重复、端口被占用。
- 生产环境（`server.env: production` → Gin ReleaseMode）**必须**配置
`server.trusted_proxies`，否则 **Start 失败**。部署在 Nginx/LB/CDN
后面填代理网段；直连对外显式写 `trusted_proxies: []`。
- 容器环境靠采集器收 stdout：把 `logs.stdout_json` 设为 `true`，否则 ANSI 颜色码会混进
日志系统；只采集文件时忽略。
- SQL 日志（错误、慢查询 >200ms）经 `pkg/logger` 落地，`record not found` 不算错误不记
ERROR；`server.env: production` 时只记 SQL 错误，慢查询要看的话把 GORM 日志级别调到 Warn。
- 从库连接失败不会阻断启动（读回落到主库），但会打 Warn 日志并触发 `alert.Notify`，收到这
条告警要去处理，不然主库在替从库扛读流量。
- `alert.Notify` 是异步的，同时在跑的 Hook 上限 **64**（`alert.MaxInFlight`），超出直接丢弃
并计数（`alert.Dropped()` 可以接进指标，示例：`GET /test/alert-dropped`）；Hook 本身应该
快速返回，不要在里面同步做重活。
- `/readyz`（或 `/health`）会在任一关键依赖不可用时返回 503，接到 k8s **readiness**；
`/livez` 接到 **liveness**，不要反接。
- 需要「多副本同一时刻只跑一次」的定时任务设 `cron.Task.Exclusive: true`（示例：
`report-low-stock`），走 Redis 锁（`LockTTL` 必须大于单次最长执行时间，没有自动续租）；
抢不到就 skipped。Redis 不可用时宁可漏跑一轮，也不要多副本同时跑。
`heartbeat` / `minute-marker` 是非互斥对照。
- Redis 连接池默认 **PoolSize=32 / MinIdleConns=4**（配置里写 0 或不写时的框架默认）。
`MinIdleConns` 不要等于 `PoolSize`，否则永远维持满池、没有突发余量。
- Redis Cluster 保持 `route_randomly: false`：开启会读从节点，幂等/限流/Exclusive 锁
可能读到滞后数据。
- 访问日志（`middleware.AccessLog`）和 HTTP 指标（`monitor.HTTPMetrics`）在示例里默认
挂在 `ContextMiddleware` 之后。生产环境用 AccessLog，不要依赖 DebugMode 下的 `gin.Logger()`。



## 深入了解

- `[docs/dev-commands.md](docs/dev-commands.md)`：本地启动、清端口、探活、常用 curl。
- `[docs/api-conventions.md](docs/api-conventions.md)`：分层/命名/校验规范，团队协作时应该遵守的规则。
- `[docs/example-apifox.openapi.json](docs/example-apifox.openapi.json)`：示例应用 HTTP 契约，导入 Apifox。
- `[pkg/frame/storage/README.md](pkg/frame/storage/README.md)`：对象存储客户端的配置项/接入指南。
- `cmd/example` + `internal/example`：完整可运行的示例应用，覆盖本文档提到的每一项能力，直接 `go run` 起来对照着看最直观。



## License

MIT
