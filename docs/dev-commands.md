# 本地命令清单

示例应用 `cmd/example` 的 How-to：启动、清端口、探活、复制即用 curl。

除非另写路径，**一律在仓库根目录**执行。框架是什么、为什么这样分层，见
[README](../README.md) 和 [api-conventions.md](api-conventions.md)。字段级契约导入
[example-apifox.openapi.json](example-apifox.openapi.json)。

HTTP 端口以你本机 `config/frame-server.yml` 的 `server.port` 为准。
`config/frame-server.yml.example` 里是 `10005`；下面命令用 `10006` 只是占位，换端口时一起改。

---

## 首次准备

```bash
# 没有本地配置时，从示例复制一份（已被 gitignore，不会提交密钥）
cp -n config/frame-server.yml.example config/frame-server.yml
```

改好 `server.port`、MySQL、Redis。主库和 Redis 连不上，进程起不来。ClickHouse / 对象存储配空或 `enabled: false` 不影响启动。

---

## 本地运行

```bash
# 推荐：按包编译（以后 main 包拆文件也不用改命令）
go run ./cmd/example

# 等价：当前 main 包只有一个文件
go run cmd/example/main.go

# 指定配置
go run ./cmd/example -c ./config/frame-server.yml
CONFIG_FILE=./config/frame-server.yml go run ./cmd/example
```

起来后日志里会有 `gin: server started` 和 `example: after start`（带 `routes`、`mysql_ready`、`redis_ready`、`clickhouse_ready`）。MySQL / Redis 没就绪会直接退出；ClickHouse 未启用不影响启动。

编译成二进制再跑（方便后台留着、也方便按进程名杀）：

```bash
mkdir -p bin
go build -o ./bin/example ./cmd/example
./bin/example
./bin/example -c ./config/frame-server.yml
```

不要在 `cmd/example` 目录里 `go run .` 然后指望默认配置还能找到：默认路径是相对**当前工作目录**的 `./config/frame-server.yml`。

---

## 探活

```bash
curl -sS -o /dev/null -w "%{http_code}\n" localhost:10006/livez    # 进程活着 → 200
curl -sS localhost:10006/readyz                                    # 关键依赖挂了 → 503
curl -sS localhost:10006/health                                    # /readyz 的兼容别名
curl -sS localhost:10006/metrics                                   # prometheus.password 为空则不用鉴权
```

---

## 端口占用 / 清理旧进程

报 `listen :10006: bind: address already in use` 时用这一节。

```bash
# 谁占了端口
ss -lptn 'sport = :10006'

# 按端口优雅停（发 SIGTERM，走框架关闭流程）
kill "$(ss -lptn 'sport = :10006' | grep -oP 'pid=\K[0-9]+' | head -1)"

# 还占着再强杀
kill -9 "$(ss -lptn 'sport = :10006' | grep -oP 'pid=\K[0-9]+' | head -1)"
```

按进程名清（本机常见是编译产物或 `go run` 拉起的进程）：

```bash
# 编译出来的二进制
pkill -TERM -f './bin/example'

# go run 拉起的示例进程（匹配命令行）
pkill -TERM -f 'cmd/example'

# 确认已经没人听 10006
ss -lptn 'sport = :10006'
```

一键：停掉占端口的旧进程，再启动：

```bash
pid="$(ss -lptn 'sport = :10006' | grep -oP 'pid=\K[0-9]+' | head -1)"
[ -n "$pid" ] && kill "$pid" && sleep 1
go run ./cmd/example
```

---

## 停服 / 信号

前台 `go run`：终端里 `Ctrl+C`（SIGINT）。

后台二进制：

```bash
# 优雅关闭：BeforeStop → 按注册反序 Stop 组件
kill -TERM "$(pgrep -n -f './bin/example')"

# SIGHUP 热重启默认关闭；没开 WithRestartSignal 时会被忽略
kill -HUP "$(pgrep -n -f './bin/example')"
```

---

## 常用接口（服务已起来）

`/test/*` 多数要带演示头 `X-Demo-Token`。`POST /api/orders` 必须带 `Idempotency-Key`，`quantity` 范围是 1–99。

`/api/products` 组挂了限流中间件：每个路由 `FullPath` × ClientIP 各 10 次/分钟，不是整组共用一个桶。超限 HTTP 429。完整字段和错误码见 OpenAPI。

```bash
# 能力演示
curl -sS -H "X-Demo-Token: x" localhost:10006/test/success
curl -sS -H "X-Demo-Token: x" localhost:10006/test/timezone

# 产品
curl -sS "localhost:10006/api/products/list?page=1&page_size=20"
curl -sS localhost:10006/api/products/summary
curl -sS localhost:10006/api/products/1

# 系统配置（config_db）。key 是 product.notice，见 internal/example/constant.ConfigKeyProductNotice
curl -sS localhost:10006/api/system-configs/product.notice
curl -sS -X POST -H "Content-Type: application/json" \
  -d '{"value":"hello"}' localhost:10006/api/system-configs/product.notice

# 定时任务
curl -sS localhost:10006/api/cron/tasks
curl -sS -X POST localhost:10006/api/cron/report-low-stock

# 下单（同一 token + 同一 Idempotency-Key 会重放同一响应）
curl -sS -X POST localhost:10006/api/orders \
  -H "Content-Type: application/json" \
  -H "X-Demo-Token: demo-user-1" \
  -H "Idempotency-Key: $(uuidgen)" \
  -d '{"product_id":1,"quantity":1}'
```

更多 curl 写在各 handler / `*_route.go` 的注释里。

---

## 日志

配置里 `logs.is_file: true` 时写到仓库根下：

```bash
tail -f logs/frame.log
```

---

## 常见失败

| 现象 | 处理 |
| --- | --- |
| `加载配置文件失败` | 确认在仓库根目录，或加 `-c` / `CONFIG_FILE` |
| `listen :10006: address already in use` | 上面「端口占用 / 清理旧进程」 |
| `after start: required deps not ready` | 查 `config/frame-server.yml` 里 MySQL / Redis 地址能否连通 |
| `/readyz` 503 | 关键依赖（示例：default_db / config_db / redis）挂了；`/livez` 仍应 200 |
