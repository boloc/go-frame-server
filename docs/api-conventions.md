# HTTP 接口命名与分层规范

> 目的：一份团队里任何人都能照抄的规范。
> 范围：只讲命名 / 分层 / 校验这些容易被新人无意打破的约定。
> 怎么启动和 curl：[`docs/dev-commands.md`](dev-commands.md)。
> HTTP 字段和错误码：[`docs/example-apifox.openapi.json`](example-apifox.openapi.json)。
> 框架总览：[`README.md`](../README.md)。

路径以 `cmd/example/route` 为准。产品列表是 `GET /api/products/list`，不是 `GET /api/products`。公告 key 是 `product.notice`，不是 `product_notice`。

---

## 1. Handler 命名：`{Resource}{Action}` 包级函数

**规则：**

- handler 是**包级函数**，不是某个 `XxxHandler` 结构体的方法；函数体内部 `logic.NewXxxLogic()` 现场 new，不做构造注入。
- 函数名格式固定为 `{Resource}{Action}`，例如 `ProductList`、`ProductCreate`、`OrderCreate`、`OrderRefund`、`SystemConfigGetByKey`。
- **禁止**使用通用 CRUD 名字（`List`/`Add`/`Update`/`Delete`）作为函数名或方法名——包级函数场景下会撞名，而且丢失业务语义（`Update` 不知道改的是什么，`OrderRefund` 一眼能看出是退款动作，出问题查日志/监控时更快定位）。
- **禁止**为了"看起来更 OOP"引入 `ProductHandler{}.List()` 这种 struct 方法 + 构造注入的写法：每加一个依赖要同时改 handler 构造函数签名和路由装配代码两个地方，跟"写代码应该循序渐进、不用一开始就想清楚完整依赖链"这条开发体验直接冲突，而 `logic`/`repository` 在这个框架里通常是无状态的薄封装，构造注入换来的可测试性收益并不明显。

```text
route      products.GET("list", handler.ProductList)  // 完整路径 GET /api/products/list
handler    func ProductList(c *gin.Context) { ... logic.NewProductLogic().GetList(&req) ... }
```

参考实现：`internal/example/handler/product_handler.go`、`internal/example/handler/order_handler.go`。

## 2. 参数校验：只用 `validate:` tag，禁止 `binding:` tag

**规则：**

- 请求结构体的字段级校验统一写 `validate:"required,min=1"` 这种 **`validate:`** tag，由 `pkg/frame/validate` 处理。
- **禁止**使用 gin 原生的 **`binding:`** tag。

**为什么要禁止 `binding:`，而不是"两个都能用随便选"：**

`webx.Bind`/`BindQuery`/`BindJSON`/`BindURI` 内部调用的是 `c.ShouldBind*`，这一步本身会触发 gin **自带的一套 validator**（默认识别 `binding:` tag），和框架 `pkg/frame/validate` 是完全独立的两个实例。如果字段上混用了 `binding:` tag：

1. gin 会在 `ShouldBind*` 这一步就报错，走 `errs.Wrap(CodeInvalidParams, err, "请求参数格式错误")` 这个通用兜底分支；
2. 这条路径**没有** `pkg/frame/validate` 那种逐字段中文说明和 `Fields` map，前端只能拿到一句笼统的"请求参数格式错误"。

也就是说混用不会报错、能跑，但会让参数校验的错误体验在不知不觉中退化，且不容易在 review 时看出来。

正确写法（唯一允许的方式）：

```go
type ProductListReq struct {
	Keyword string `form:"keyword" validate:"omitempty,max=50"` // 只用 validate:，不写 binding:
}
```

**这条规则配套了 AST 检查**，见第 4 节。

### 2.0 绑定入口怎么选，以及 `BindWithQueryFallback` 这个例外

正常情况下按参数来源选一个就行：`Bind`（按 Content-Type 自动选）、`BindQuery`、`BindJSON`、`BindURI`。四个都会在绑定成功后自动跑一次 `validate:` tag 校验。

`BindWithQueryFallback` 是第五个，行为是「先按 Content-Type 正常绑定，body 解析失败时再回退读一次 query」。**它只给历史客户端兼容用，新接口一律不要用。**

存在的原因：部分手机浏览器（iOS 上的 QQ 浏览器是常见一例）在页面跳转场景下会把 POST body 丢掉，只剩 URL 上的 query。这类客户端已经发出去了改不动，服务端只能两边都认。

不要把它当默认写法的理由：

- 参数到底从 body 来还是从 query 来，本该是接口契约的一部分。「哪个能解出来算哪个」会让契约变模糊，联调时很难说清。
- 它会把客户端真正的请求格式错误掩盖成一次静默回退，问题推迟到更靠后的地方才暴露。
- body 解析失败时请求结构体可能已经被填了一半，回退的 query 绑定只覆盖 query 里出现的字段，残留值会保留。这是这个兼容本身固有的模糊之处，没法消除。

所以用它的地方要在 handler 上写明为什么用，review 时按「有没有真实的历史客户端」来判断，不接受「顺手写上更保险」。

### 2.1 dto 不实现 `validate.Validatable`，跨字段规则一律放 `validation` 包

`pkg/frame/validate` 除了 tag 校验，还提供了一个 `Validatable` 接口（`Validate() error`），请求结构体实现它之后会被 `webx.Bind` 系列**自动**调用（不需要 handler 显式写调用点）。框架能力演示见 `POST /test/validatable`（结构体活在 handler 文件里，**不**在 `dto` 包）。

**项目约定：业务 dto 不实现这个接口。** 原因：

- 校验分两层是为了"职责明确"——tag 是纯声明式的字段格式校验（自动执行，不需要写代码逻辑）；跨字段/业务规则应该有一个统一、显式、可单独单测的落脚点。
- 如果 dto 里再实现 `Validate()`，会出现"tag 之外还有一层校验，也在绑定阶段自动跑，但代码却混在 dto 文件里"的模糊地带——校验逻辑到底该去 dto 里的 `Validate()` 找，还是去 `validation` 包找，团队里会有人记不住。
- `internal/example/dto/order_dto.go` + `internal/example/validation/order_validation.go` 就是唯二两层职责的真实示例：`OrderCreateReq` 只有 tag，没有 `Validate()`；"该产品是否已下架"这种跨字段/业务规则显式写在 `ValidateOrderCreate`，由 `handler.OrderCreate` 显式调用一次。这是本项目业务接口唯一认可的模式。

| | `validate:` tag | `validate.Validatable` | `internal/<domain>/validation` |
|---|---|---|---|
| 谁调用 | `webx.Bind*` 自动 | `webx.Bind*` 在 tag 通过后自动 | handler **显式**调用 |
| 适合 | 单字段格式 | 框架能力（演示接口可以） | 业务跨字段/运营规则（不查库） |
| 业务 dto | 必须用 | **禁止** | 必须用这一层 |

```go
// dto：只放 tag
type OrderCreateReq struct {
	ProductID uint `json:"product_id" validate:"required"`
	Quantity  int  `json:"quantity" validate:"required,min=1,max=99"`
}

// validation：跨字段/业务规则，显式调用
func ValidateOrderCreate(req *dto.OrderCreateReq) error {
	if bannedProductIDs[req.ProductID] {
		return errs.InvalidParams("该产品已下架，暂不支持购买")
	}
	return nil
}
```

需要查库才能判断的规则（库存够不够之类）不放 `validation` 包，属于 `logic`/`repository` 层职责。

## 3. Model / DTO 两层，不引入 Entity 层

**规则：**

- 数据只有两种形态：`internal/model`（GORM 持久化结构）和 `internal/<domain>/dto`（HTTP 请求/响应契约）。**不引入** `entity`/`VO` 这类中间层。
- `dto` 包按"域"划分（`product_dto.go`、`order_dto.go`），不按"请求/响应"拆目录；同一个文件内用注释分区：

```go
// ==================== 请求 ====================
type ProductListReq struct { ... }

// ==================== 响应 ====================
type ProductItem struct { ... }
```

- `model` → `dto` 的转换用 `FromModel` 方法一次到位（`dto.ProductItem.FromModel(m *model.Product)`），不经过任何中间结构体。
- 查询条件（Search Condition）等"中性"结构放在 `dto` 包里，不要放进 `repository` 包，避免 `dto` 反向依赖 `repository`。

**什么时候才需要考虑加中间层**：只有当同一份数据要喂给形态差异很大的多个输出（比如同一个资源，管理端要嵌套关联对象、客户端只要扁平字段），或者存储类型和 API 类型差异大到需要独立单测的程度，才评估是否需要转换的中间结构；纯 CRUD 场景不需要。

**什么时候该拆文件（而不是拆目录/加层）**：单个 `xxx_dto.go` 文件明显变长、请求/响应类型很多时，拆成 `xxx_req.go` + `xxx_resp.go` 两个文件，包名 `dto` 不变。

参考实现：`internal/example/dto/product_dto.go`（`ProductItem.FromModel`）、`internal/example/dto/order_dto.go`。

## 4. Review 时盯住这两条，不要另发明检查脚本

第 2 节和第 2.1 节没有自动化测试兜底，靠 review 挡住：

- 请求结构体只用 `validate:` tag，不要写 `binding:`。
- `internal/**/dto` 不要实现 `func (T) Validate() error`。跨字段规则放 `validation` 包，由 handler 显式调用。

---

## 5. 幂等保护：`Idempotency-Key` 和 `X-Request-Id` 不是一回事，不要混用

**规则：**

- 创建订单、支付、退款这类"客户端网络重试时绝对不能重复执行"的接口，必须挂
  `pkg/frame/idempotency.Middleware(...)`，按路由/分组挂载（不要做成全局中间件）。
- 金融/支付类接口用 `idempotency.WithRequired(true)`，强制要求客户端传 `Idempotency-Key`
  请求头；内部管理后台一类调用方不想每次生成 key 的接口，可以不设 `Required`（默认放行）。
- **禁止**用 `reqctx.RequestContext.RequestID`（`X-Request-Id`）顶替幂等键。

**为什么这两个概念不能混用：**

| | `X-Request-Id` / `RequestID` | `Idempotency-Key` |
|---|---|---|
| 语义 | "这一次 HTTP 调用" | "这一次业务操作意图" |
| 生成方 | 客户端没传时，框架（`ContextMiddleware`）会自动生成一个 | 只能由客户端生成，框架不生成、不代替 |
| 多次重试是否应该相同 | 通常不需要——每次调用可以是新值，只用于日志/链路追踪串联 | 必须相同——服务端靠这个值判断"这是不是同一个操作的第 N 次尝试" |
| 用途 | 排查问题时"对着这个值查日志" | 防止创建订单/支付类接口被重复执行 |

如果拿 `RequestID` 当幂等键用，客户端一旦按"每次调用生成新 ID"的正常语义重试，幂等保护会
直接失效（每次都是新 key，等于没做保护）；反过来拿 `Idempotency-Key` 当追踪 ID 用，会导致
同一个业务操作的多次重试在日志里全部长得一样，没法区分"这是第几次重试"——两者职责不同，
必须分开维护，见 `pkg/frame/reqctx/context.go` 里 `RequestID` 字段的注释。

**中间件核心行为（详见 `pkg/frame/idempotency` 包文档）：**

- 用 Redis `SETNX` 原子抢占幂等键，避免并发重复请求同时执行两次 handler。
- 本框架 `webx.Fail` 默认永远 200，幂等中间件判断"要不要缓存这次响应"时不能只看 HTTP 状态码，
  而是解析响应体的 `code` 字段：`code < errs.CodeInternal`（成功或明确的业务/参数错误）才缓存
  重放，`code >= errs.CodeInternal`（数据库/依赖/超时等基础设施抖动）释放锁允许真正重试。
- **Redis 不可用时的降级策略必须显式选择，不能隐藏在实现细节里**（`Options.FailOpen` /
  `WithFailOpen`）：默认 `true`（fail-open，降级为不做保护直接放行，同时打警告日志），一般
  业务接口够用；**金融/支付类接口应该显式 `WithFailOpen(false)`**（fail-closed，直接拒绝请求）
  ——Redis 故障往往和"客户端疯狂重试"同时发生，这恰好是幂等保护最该生效、也最容易被
  fail-open 悄悄绕过的时刻，宁可这段时间接口不可用，也不能被绕过重复下单/重复支付。

参考实现：`cmd/example/route/order_route.go` 的 `/api/orders`（`WithRequired(true)` +
`WithFailOpen(false)` + `WithScopeFunc`）。

### 5.1 幂等键的作用域

默认 Redis key = `KeyPrefix` + 路由 `FullPath` + `:` + 客户端 `Idempotency-Key`，其中
`KeyPrefix` 默认是 `rediskey.Prefix(rediskey.SegIdempotency)`。框架四类 Redis key 的完整
组成都记在 `pkg/frame/rediskey` 的包文档里，那里是唯一出处，见下面第 14 节。

- 客户端用全局唯一 UUID，或接口没有用户概念时，**不需要** `WithScopeFunc`。
- 键可能跨用户重复（大家都传 `"1"`），或要防止别人猜键重放响应时，用
  `idempotency.WithScopeFunc` 把用户标识拼进 key：
  `KeyPrefix + FullPath + ":" + scope + ":" + 幂等键`。`ScopeFunc` 返回空字符串时
  key 格式不变。

示例用 `X-Demo-Token` 当演示用户：同一 token + 同一 `Idempotency-Key` 重放；换 token
同 key 互不干扰。见 `cmd/example/route/order_route.go` 顶部的两条 curl。

## 6. 限流：Redis 不可用时的降级策略同样必须显式选择

**规则：**

- 需要防刷/防滥用的接口，按路由/分组挂载 `pkg/frame/ratelimit.Middleware(...)`（不要做成
  全局中间件——不是所有接口都需要限流，额外的 Redis 往返成本不该让不需要的接口也付）。
- `WithLimit`/`WithWindow` 必须显式设置，没有通用的默认阈值，不设置会在路由注册阶段直接
  panic（这是启动期就该发现的编程错误）。
- Redis 故障时的降级策略同样必须显式选择（`WithFailOpen`），语义和默认值跟
  `pkg/frame/idempotency` 完全一致：`true`（默认）降级放行，`false` 直接拒绝。限流用来
  保护一个脆弱的下游依赖时（下游扛不住超过阈值的流量），应该显式 `WithFailOpen(false)`，
  宁可拒绝也不能让 Redis 故障变成"限流形同虚设、下游被打垮"的连锁故障；纯粹的防刷场景用
  默认值就够。
- 默认 `UseRealStatus=true`：超限返回真实的 HTTP 429 + `Retry-After` 响应头，**不遵循**
  本框架"`webx.Fail` 默认永远 200"的一般约定——限流是专门设计给网关/负载均衡器/客户端
  退避逻辑识别的信号，这些组件普遍认 HTTP 状态码，不会去解析业务响应体里的 `code`。
  如果前端确实只处理 200 + body.code，用 `WithUseRealStatus(false)` 切回 `webx.Fail`。

参考实现：`cmd/example/route/product_route.go` 的 `/api/products` 组。中间件挂在组上，默认 Redis key 是 `rediskey.Prefix(rediskey.SegRateLimit)` + 路由 `FullPath` + `:` + ClientIP（见 `pkg/frame/ratelimit` 的 `defaultOptions` + `defaultKeyFunc`），所以 `/list`、`/summary`、`/:id` 各自 10 次/分钟，不是整组共用一个桶。

## 7. 定时任务：任务只负责"业务逻辑"，调度相关的关注点都交给 `pkg/frame/cron`

**规则：**

- 定时任务统一用 `pkg/frame/cron.Component` 注册（`cron.NewComponent(name, tasks...)` +
  `f.RegisterComponent(...)`），不要自己起 goroutine + `time.Sleep`/`time.Ticker` 写调度，
  也不要在业务代码里自己监听 `SIGINT`/`SIGTERM`——"进程什么时候退出"只应该由
  `frame.Frame.RunContext` 一个地方决定，定时任务只需要在 `Task.Run(ctx)` 里响应
  `ctx.Done()`。
- **业务侧只应该有一个"顶层"`cron.Component`**（比如 `cmd/example/cron.Component`），
  新增业务定时任务是往已有的 `NewComponent(...)` 调用里加一个 `Task`，不是新建第二个
  顶层 `Component`。`pkg/frame/refreshcache.Cache` 是唯一的例外，它内部每个实例都各自
  持有一个独立的 `cron.Component`。`NewComponent(name, tasks...)` 的 `name` 参数用来
  区分同一进程里的多个 `Component`（避免 Prometheus 指标同名冲突）：业务顶层 Component
  传一个稳定的应用名（比如 `"example"`），`refreshcache.Cache` 内部用它自己的 `Key`。
- 注册这类组件用 `f.RegisterSingleton(key, component)`，**不要**用 `f.RegisterComponent`
  ——两者的区别：`RegisterComponent` 允许同一个 key/类型注册任意多次（MySQL/Redis 那种
  故意支持多实例的场景该用它）；`RegisterSingleton` 要求 key 唯一，重复注册**直接 panic**，
  报错发生在调用的那一行，不用等到 Prometheus 重复注册指标时才炸出一个更难定位的错误
  （两个 `cron.Component` 的指标名是硬编码的、不带实例标签，重复注册必然冲突）。
  `cmd/example/bootstrap/cron.go`/`refreshcache.go` 已经是这个写法。
- `Task.Run` 内部如果有可以被取消的耗时操作（数据库查询、HTTP 调用……），要把 `ctx` 一路
  传下去——`Component.Stop` 被调用时会取消这个 `ctx`，如果任务内部不理会它，"优雅关闭"
  只是变成"调度器不再触发新的一轮，但已经在跑的这一轮不管多久都会跑完"，不会真正响应
  关闭信号。
- 任务名（`Task.Name`）在同一个 `Component` 内必须唯一，重复会在 `Start` 阶段直接报错。
- 需要监控定时任务执行情况时，用 `Component.Collectors()` 显式注册进 Prometheus
  （`cron_task_runs_total{task,status}` / `cron_task_duration_seconds{task}`），不要
  自己再发明一套指标命名。
- **要不要跑定时任务应该做成配置项**，不是所有环境都需要（比如多副本部署，通常只想让
  一个副本/一个单独的 worker 角色跑定时任务）。这条决策放在 bootstrap 层（读配置，决定
  要不要调用 `RegisterSingleton`），不要下沉进 `pkg/frame/cron` 本身——`cron.Component`
  不应该知道"配置"这个概念，跟 MySQL/Redis 组件本身不关心业务配置格式是同一个原则。
  默认启用：只有显式配成 `false` 才关闭。用 `viper.IsSet` 判断"有没有配置"而不是直接
  读 `GetBool` 的结果，否则"没配置"和"显式配成 false"会被误判成同一种情况。参考实现：
  `cmd/example/bootstrap/cron.go` 的 `cronEnabled`/`SetupCron`，配置项见
  `config/frame-server.yml` 的 `cron.enabled`。

参考实现：`cmd/example/cron/cron.go`（任务列表单例）+ `cmd/example/bootstrap/cron.go`
（注册进 Frame）。任务列表单例放在 `cmd/example/cron` 而不是 `internal/example`：它的
性质跟 `route.go` 一样是"接线"（名字/调度表达式 -> 该调用谁），不是业务逻辑，业务逻辑
仍然在 `internal/example/repository`/`logic` 里；只有真正需要查库/绑参数这类业务动作
的 handler（比如手动触发库存检查）才留在 `internal/example/handler`。

**手动执行某个定时任务（运维常见需求，对照 nav-market-c 的"手动执行: xxx"系列管理接口）**：
不要给框架加一个"按任务名字反查再触发"的能力，直接在 handler 里调用那个任务背后真正的
repository/logic 方法——`Task.Run` 本来就是一个独立可调用的方法值/闭包，手动触发和定时
触发用同一份代码，不会出现两条路径行为不一致的风险。`Component.Tasks()` 只用来给"查看已
注册任务"这类只读的管理接口用（返回 Name/Schedule/Exclusive/LockKey，不暴露 `Run`），这类纯接线内省接口
放在 `cmd/example/cron.TaskListHandler()`；需要调业务逻辑的手动触发接口
（`internal/example/handler/cron_handler.go` 的 `CronManualReportLowStock`）留在
`internal/example/handler`，跟其它业务 handler 一致——两者放在不同包，不是疏漏，
是"接线内省 vs 业务逻辑"两种不同职责的自然结果。参考路由：
`GET /api/cron/tasks` + `POST /api/cron/report-low-stock`。

### 7.1 `Exclusive`：多副本下同一时刻只跑一次

需要「全局只跑一次」的任务设 `Task.Exclusive: true`（示例：`report-low-stock`），
执行前抢 Redis 锁（`rediskey.Prefix(rediskey.SegCronLock) + <scheduler> + ":" + <task>`，
`SET NX PX`）。`<scheduler>` 是 `NewComponent` 的 `name`，`<task>` 是 `Task.Name`；不想靠
拼字符串反推时用 `Component.LockKey(taskName)`，`Component.Tasks()` 也会直接返回每个
Exclusive 任务的 `LockKey`。抢不到记
`status=skipped`，打 Info，不告警。`LockTTL` 必须大于单次最长执行时间，没有自动续租。

Exclusive 任务通常不幂等：Redis 不可用时宁可漏跑一轮（skipped + Warn + alert），
也不要多副本同时跑。每个副本都该跑的任务（心跳、本机探活）保持 `Exclusive: false`，
示例里 `heartbeat` / `minute-marker` 就是对照。

不要再在 `Run` 里自己套一层 Redis 锁——框架已经做了。要不要跑整个调度器仍由
`cron.enabled` 在 bootstrap 决定。

## 8. 中间件顺序：谁必须排在谁前面，为什么

全局中间件与分组中间件的先后顺序如下，业务按本节装配即可。

### 8.1 `GinComponent` 内置的默认顺序（业务不需要手动排）

`components.NewGinComponent(...)` 内部已经按下面这个顺序注册好了，业务只需要用
`WithGinXxx` Option 调整开不开、阈值多少，不需要关心顺序：

```text
1. gin.Logger()              仅 DebugMode 开启
2. gin.CustomRecoveryWithWriter  兜底 panic，堆栈经 pkg/logger 落地（不用 gin.Recovery，避免只打到 stderr）
3. middleware.MaxBodyBytes    默认 4MB，WithGinMaxBodyBytes(0) 可关闭
4. middleware.RequestTimeout  默认关闭，WithGinRequestTimeout(d) 开启
5. detectUnconfiguredProxy    仅 TrustedProxies 未配置时开启（提醒用，不拦截请求）
6. WithGinMiddleware(...) 里传入的中间件
```

**第 3 步必须排在任何会读整包 body 的中间件之前**（比如 8.2 的 `ContextMiddleware`）：
`MaxBodyBytes` 用 `http.MaxBytesReader` 包一层 `c.Request.Body`，如果读 body 的中间件先跑，
限制形同虚设——`NewGinComponent` 内部的注册顺序已经保证了这一点，业务只有在**脱离
`GinComponent` 自己拼 `gin.Engine`** 时才需要自己操心这条规则。

### 8.2 业务在 bootstrap 阶段追加的顺序

```71:76:cmd/example/bootstrap/gin.go
	ginComponent.Use(
		middleware.ContextMiddleware(),
		middleware.AccessLog(middleware.WithSlowThreshold(time.Second)),
		monitor.HTTPMetrics(),
	)
	f.RegisterComponent(ginComponent)
```

`ginComponent.Use(...)` 在 `NewGinComponent` **返回之后**才调用，所以 `ContextMiddleware`、
`AccessLog`、`HTTPMetrics` 排在上面 8.1 全部六步**之后**——这正好满足"必须排在
`MaxBodyBytes` 之后"的要求。`AccessLog` 和 `HTTPMetrics` 要读 `ContextMiddleware` 写入的
`request_id` / `ClientIP`，以及 handler 返回后的 `webx.BizCode`，必须排在
`ContextMiddleware` 之后、业务路由之前。

**排序结论：**

```text
MaxBodyBytes → RequestTimeout → detectUnconfiguredProxy → ContextMiddleware → AccessLog → HTTPMetrics
```

如果业务还要加别的全局中间件（鉴权、审计日志……），一律用 `ginComponent.Use(...)`
在 `SetupGin` 里追加，会自然排在上述链之后——**除非这个中间件也需要读
body**（这种情况很少见，读 body 的需求几乎都应该走 `reqctx.FromGin(c).RequestBody`，
不需要再读第二次）。

### 8.3 路由/分组级中间件（`idempotency`/`ratelimit`/业务鉴权……）

这些不挂在 `GinComponent` 全局链上，而是 `r.Group(...).Use(...)`（见
`cmd/example/route/order_route.go`、`product_route.go`）。Gin 的执行顺序是
"全局中间件全部先跑完，再跑命中的分组中间件"，所以**分组中间件天然排在
`ContextMiddleware` 之后**，不需要业务操心顺序——这也是为什么
`pkg/frame/ratelimit` 的 `defaultKeyFunc` 敢直接读 `reqctx.FromGin(c).ClientIP`：
等分组中间件跑到的时候，`ContextMiddleware` 已经把这个字段填好了（即便没填好，
`defaultKeyFunc` 也有 `c.ClientIP()` 兜底，见 `pkg/frame/ratelimit/ratelimit.go`）。

同一个分组内挂多个中间件时，**先 `.Use()` 的先执行**。`/api/orders` 的顺序是
`RequireDemoToken` 再 `idempotency.Middleware`（先确认请求"是谁"，再判断这个人的操作
是否重复提交）。鉴权示例见 `internal/example/middleware/demo_token.go`（`RequireDemoToken`
+ `DemoUser`）；幂等 scope 用 `idempotency.WithScopeFunc` 读取 `DemoUser`，见
`cmd/example/route/order_route.go`。

### 8.4 一张总表

| 顺序 | 中间件 | 挂载方式 | 前置要求 |
|---|---|---|---|
| 1 | `gin.Logger()` / `gin.CustomRecoveryWithWriter` | `GinComponent` 内置 | 无 |
| 2 | `middleware.MaxBodyBytes` | `GinComponent` 内置 | 必须在任何读 body 的中间件之前 |
| 3 | `middleware.RequestTimeout` | `GinComponent` 内置 | 无强制要求，建议靠前 |
| 4 | `detectUnconfiguredProxy` | `GinComponent` 内置 | 无 |
| 5 | `middleware.ContextMiddleware` | `bootstrap.SetupGin` 显式 `Use` | 必须在 `MaxBodyBytes` 之后 |
| 6 | `middleware.AccessLog` / `monitor.HTTPMetrics` | `bootstrap.SetupGin` 显式 `Use` | 必须在 `ContextMiddleware` 之后（读 request_id / ClientIP / BizCode） |
| 7 | 业务鉴权/审计类全局中间件 | `bootstrap.SetupGin` 显式 `Use` | 建议在 `ContextMiddleware` 之后（如果要用 `reqctx`） |
| 8 | 分组鉴权中间件（示例：`RequireDemoToken`） | `group.Use(...)` | 建议排在幂等/限流之前 |
| 9 | `idempotency.Middleware` / `ratelimit.Middleware` | `group.Use(...)` | 依赖 `reqctx.ClientIP`（有兜底），不强制要求 `ContextMiddleware` 已跑 |

## 9. 统一失败通知：`pkg/alert`

**规则：**

- 需要接入外部通知渠道（飞书/企业微信/PagerDuty……）时，进程里调用**一次**
  `alert.SetHook(func(ctx context.Context, e alert.Event) { ... })`（示例在
  `cmd/example/main.go`，`bootstrap.Setup` 之后、`Run` 之前），在这一个函数内部
  按 `e.Scope` 分发到不同渠道，不要在框架各个子系统里分别接一套通知逻辑。
- `alert.Notify` 已经接在这些位置，注册一次 Hook 就能全部覆盖：`pkg/frame/frame.go`
  （组件启停失败、`AfterStart`/`BeforeStop` 钩子失败）、`pkg/frame/components/gin.go`
  （HTTP 服务器后台 Serve 循环意外退出）、`pkg/frame/cron`（任务失败/panic）、
  `pkg/frame/idempotency`/`pkg/frame/ratelimit`（Redis 降级发生时）、`pkg/frame/webx`
  （`>= errs.CodeInternal` 的请求失败）。
- `webx.SetOnFail` 不受影响，继续按它原来的方式用（参数是 route + `*errs.Error`，比
  `alert.Event` 更精确，适合做请求级别的错误率统计）；`alert` 是更泛化的"后台/异步也要
  覆盖"的那一层，两者不冲突，可以同时用。
- `alert.Notify` 是 fire-and-forget（内部另起 goroutine + recover），业务的 Hook 函数
  即使自己 panic 或者调外部服务很慢，也不会拖慢/搞崩调用它的那个子系统（cron 任务、
  HTTP 请求……）；但也正因为是异步的，Hook 内部不要依赖"这次调用一定会在进程退出前
  执行完"，重要的通知应该有自己的重试/落盘机制，不要假设 `alert` 提供了这个保证。

## 10. 双层刷新只读缓存：`pkg/frame/refreshcache`

**规则：**

- 只用于读多写少、允许有界延迟的数据（运营配置、下拉选项、导航栏列表……），**不要**
  用于要求强一致的读写场景（库存扣减之类，请用真实的数据库事务/Redis 原子操作）。
- `Key`/`Loader`/`RedisInterval`/`MemoryInterval` 必须显式设置，缺一个时 `Start`
  返回 error（`New` 本身不检查、不 panic）；`MemoryInterval` 通常应该比 `RedisInterval`
  短（内存层刷新成本几乎为零，可以刷得更频繁）。
- 多实例部署时用 `Jitter` 给刷新时机加一点随机抖动，避免所有实例在同一个调度边界一起
  打数据源/Redis。
- `Get()` 优先读内存，只有"从来没有成功加载过"时才会同步兜底调一次 `Loader`——正常运行
  期间 `Get()` 永远是纯内存读，没有任何网络往返。

**和 `pkg/frame/cron` 的关系（这是设计上最容易被问到的一点）**：`refreshcache.Cache[T]`
内部**用 `cron.NewComponent` 调度**两条刷新链路，不是重新发明一套定时器；cron 只解决
"什么时候跑"，`refreshcache` 补的是 cron 不管的三件事——类型安全的内存存储容器、
"从来没加载过"时的同步兜底、以及"Redis 不可用时内存层直接回退到数据源"这条三层回退逻辑。
单独用 cron 搭不出这个模式（没有存储/回退这层职责），但也不需要给 `refreshcache` 再写
一套独立的调度代码——组合优于重新发明，这也是为什么 `refreshcache.Cache[T]` 直接把
`Collectors()` 转发给内部的 `cron.Component`，两个包共用同一套执行指标格式。

参考实现：`pkg/frame/refreshcache/refreshcache.go` 包文档；真实业务场景演示见
`internal/example/cache/product_cache.go`（缓存"当前上架产品数量"）+
`cmd/example/bootstrap/refreshcache.go`（注册进 Frame）+ `GET /api/products/summary`
（`internal/example/handler/product_handler.go` 的 `ProductActiveCountSummary`，读路径
是纯内存读，不查库）。

**`Task.Run`/`Cache.Loader` 要不要包一层匿名函数**：只在方法签名跟框架要的类型
（`func(ctx context.Context) error`）刚好一致时才直接引用方法值（见
`cmd/example/bootstrap/cron.go` 的 `report-low-stock` 任务，直接传
`repository.DefaultOrderRepository().ReportLowStock`，没有包任何匿名函数）；签名不匹配
时才需要一层薄的适配闭包（`internal/example/cache/product_cache.go` 的 `Loader` 就是
这种情况——`ProductRepository.CountActive` 的签名跟 `Loader` 一致，但它需要先
`repository.NewProductRepository()` 现场取实例，这一步"取实例"才是闭包存在的原因，
闭包内部本身没有写任何业务规则）。不对应真实业务方法、纯粹演示框架能力的任务
（`heartbeat`/`minute-marker`）才用匿名函数内联写。

## 11. 数据库迁移：AutoMigrate 只用于本地/开发，生产环境的破坏性变更走版本化 SQL

**先说开关：**

- 是否执行自动迁移由 `database.auto_migrate` 配置项决定（`config/frame-server.yml`），
  **默认关闭**——跟 `cron.enabled` 默认开启故意相反：定时任务默认开着代价很低（最多多打
  几行心跳日志），自动迁移哪怕只是"新增列"也是会动生产表结构的操作，不该在没人明确
  决定的情况下默认发生。这也是 nav-market-c 真实生产的做法（`database.is_migrate`
  默认 `false`，只有 admin 服务显式打开）。
- 实际迁移动作注册成独立 Component（`SetupMigration`），排在 MySQL `Start` 之后、
  缓存 `Start` 之前。不要放进 `AfterStart`：缓存预热发生在组件 `Start` 里，AfterStart
  太晚。迁移失败会让这个 Component 的 `Start` 返回 error，`Frame` 回滚已启动的组件，
  不会出现"迁移失败了，但进程好像还活着"这种状态。参考
  `cmd/example/bootstrap/migration.go`。
- 模型该迁移到哪个命名 MySQL 实例，必须显式声明（`internal/migration/migration.go` 的
  `perInstanceModels`），不能假设所有模型都在同一个库——这个示例项目里
  `model.SystemConfig` 就存在 `config_db`，不是 `default_db`。

**再说"SQL 还是 model"——这不是二选一，是"看环境、看变更类型"：**

| | GORM `AutoMigrate`（model 驱动） | 版本化 SQL 迁移（`golang-migrate`/`goose` 之类） |
|---|---|---|
| 能做什么 | 新增表、新增列、新增索引 | 任意 DDL：改列名、改类型、删列、多步骤的 expand-contract |
| 不能做什么 | **不会**重命名列（改了 struct 字段名会变成"新增一列"，旧列留着不动）、**不会**删除列/表、**不会**收窄类型 | 没有限制，但要自己写对 |
| 历史/回滚 | 没有——每次都是"当前 struct 和当前表结构做 diff" | 有迁移历史表，大部分工具支持 `up`/`down` |
| Review 方式 | 看 Go struct 的 diff，猜它会生成什么 DDL | 看 `.sql` 文件的 diff，DDL 是什么一目了然 |
| 多实例并发风险 | 多个副本同时启动、同时触发 AutoMigrate，对同一张表并发 DDL 有锁冲突风险 | 通常作为独立步骤跑一次（CI/CD 里、或者手动跑），不跟应用启动绑在一起 |
| 适合的场景 | 本地/开发环境快速迭代，加表加列这类纯增量变更 | 任何要上生产环境的变更，尤其是改列名/类型/加约束这类破坏性变更 |

**结论：**

- **本地/开发环境**：开 `database.auto_migrate: true`，改完 model 直接重启进程就有最新表
  结构，不需要手写 SQL、不需要额外工具。本示例的 AutoMigrate 就是为这个场景准备的。
- **生产环境**：不建议依赖 `AutoMigrate`。原因不是"AutoMigrate 有 bug"，是它的能力边界
  本身就不覆盖破坏性变更——一旦需要改列名/改类型/加唯一约束到已有脏数据的表，
  `AutoMigrate` 要么什么都不做（悄悄留下一个孤儿列），要么直接报错，两种结果都不是你想要
  的。这类变更应该走版本化 SQL 迁移工具，作为部署流程里独立的一步（在应用新版本上线
  之前跑，不是跟着应用启动自动跑），配合"expand-and-contract"的分阶段发布思路
  （先加新列/双写，确认没问题再删旧列，不是一次性切换）。
- **框架现状**：只提供 `AutoMigrate`，暂不引入版本化迁移工具；破坏性变更用一份 review
  过的 SQL 脚本手动跑一次。

## 12. 枚举下拉选项：`pkg/util/options`

管理后台"下拉选项 + 状态文案"这类枚举场景统一用 `options.NewBuilder[K,V]()`，一次 `Put`
同时拿到有序展示列表（`Options()`）和查找表（`Get`/`Contains`/`Map`），不要分别手写
`switch`（列表页文案）和 map（下拉框数据）两份容易漏改的重复逻辑。

枚举定义本身放在按应用新建的 `enum` 包（如 `internal/example/enum`），不放 `dto`——
枚举描述的是"某个字段合法取值有哪些"，是独立于任何请求/响应的领域知识，`dto` 只应该
描述数据形状；也不放 `model`（不该让持久化 schema 依赖"怎么展示"）或部署相关的
`constant` 包。参考实现：`internal/example/enum/product.go` 的 `ProductStatus` +
`GET /api/products/options`。

参考实现：`internal/migration/migration.go`（模型列表 + `AutoMigrate`）+
`cmd/example/bootstrap/migration.go`（配置开关 + 迁移 Component）。

## 13. 组件访问器：启动期 / 请求期 / 可选依赖

`pkg/frame` 和 `pkg/frame/components` 对 MySQL / Redis / ClickHouse 同时提供 panic 版
（`DefaultDB`、`MasterDB`、`GetRedisCmdable`……）和 Try 版（`TryDefaultDB`、
`TryGetRedisCmdable`、`components.TryDefaultClickHouseDB`……）。两套都是故意留下的，
按依赖是不是「这个进程活着就必须连着」来选，不要在 README 和业务代码里各说一套。

| 场景 | 用哪套 | 为什么 |
|---|---|---|
| 启动装配、健康检查、可选组件探测 | `Try*` | 没连上返回 `(nil, false)`，由调用方决定跳过、降级或让 Start 失败 |
| 请求期 repository / logic，且该依赖是服务运行的前提（本示例的 MySQL） | panic 版 | 服务期连着是不变量；真断了 Gin recover 成 500，比每个方法写 `if !ok` 更吵、更短 |
| 请求期可选能力（ClickHouse、对象存储、按配置关闭的子系统） | `Try*` | 未启用必须回业务错误，不能 panic。见 `POST /test/clickhouse/events`、`/test/storage/*` |

本示例 `ProductRepository` / `SystemConfigRepository` 用 panic 版取连接，是符合上表
第二行的：这三个 MySQL 实例启动失败进程不会起来，请求期再判一次 `ok` 没有信息量。
`AfterStart` 里用 `TryDefaultDB` / `TryGetRedisCmdable` 做一次就绪校验（见
`cmd/example/main.go`），是第一行。

健康检查必须用 Try：`healthcheck.MySQLChecker(frame.TryDefaultDB)` 这类探测在依赖
还没起来或已经摘掉时不能把自己 panic 掉。

## 14. Redis key：命名空间由应用注入，框架不给默认值

**规则：**

- 框架写 Redis 的能力有四类——幂等、限流、Exclusive 定时任务锁、双层刷新缓存。它们的 key
  一律按 `命名空间 + 能力段 + 实例标识` 三段拼，段定义和完整组成只记在 `pkg/frame/rediskey`
  的包文档里，那里是唯一出处。**不要在业务代码里自己拼 key 字符串，也不要在别处重复记一份
  格式说明**——这种说明一定会和代码漂移（改这版之前 `cmd/example/cron/cron.go` 的注释里就
  还写着早已改掉的调度器名）。
- **命名空间必须由应用在装配阶段用 `rediskey.SetNamespace` 注入一次，框架故意不提供默认
  值。** admin / api 这类多个应用共用同一个 Redis db 时，一个写死的默认值会让它们的键空间
  悄悄重叠：同名路由的限流会共用一个桶、同名缓存会互相覆盖，而这类问题只在线上暴露。没注入
  就让它启动不了，比给个"看起来能跑"的默认值安全。
- 命名空间取 `server.name`（参考实现：`cmd/example/bootstrap/redis.go`），所以
  `server.name` 是必填项，多个应用之间不能重名。换句话说，隔离粒度是"应用"而不是"二进制"：
  两个进程如果**故意**要共用缓存或限流桶，就该配同一个 `server.name`。
- `SetNamespace` 只能调用一次，重复调用或传空值直接 panic。要给单个挂载点换前缀用各自的
  `WithKeyPrefix`，不要试图改全局命名空间。

**为什么能在包级变量之后才注入：**

四类 key 全是懒拼的——幂等和限流在 `Middleware()` 里取前缀，而路由注册发生在
`GinComponent.Start` 内部；任务锁和缓存 key 在真正读写 Redis 时才拼。这三个时机都晚于配置
加载，所以即便 `cmd/example/cron.Component` 和 `internal/example/cache` 里的实例是包级变量
（构造早于配置），在 `bootstrap.Setup` 里注入一次就能全局生效，不需要把它们改成在 bootstrap
里构造。

**忘了注入会怎样：**

不会退化成静默共用 key，各能力都在启动期挡住——`refreshcache.Cache.Start` 和
`cron.Component.Start`（有 Exclusive 任务时）返回启动错误，`idempotency.Middleware` /
`ratelimit.Middleware` 在路由注册时 panic，和它们对 `WithLimit` 缺失的处理一致。

**业务自己读写的 key 不在上面四类里，框架不碰它。** `frame.GetRedisCmdable().Set(ctx, "foo", ...)`
落到 Redis 上就是 `foo`，一个字符都不会加。所以业务 key 默认是全共享的：同一个 Redis db 下
两个应用用了同名 key 就是同一份数据，没人挡。要和其它应用隔开，在调用处显式用
`rediskey.App(segments...)` 拼（`Namespace()` + 各段用冒号连接）。演示接口
`GET /test/redis-key`：`rediskey.App` 拼 key 后 `SET` 进 Redis，响应里带回真实 key。

**查真实 key：** `cron.Component.LockKey(taskName)` 返回某个任务的锁 key，
`Component.Tasks()` 会直接带上每个 Exclusive 任务的 `LockKey`（`GET /api/cron/tasks` 能看到）；
`refreshcache.Cache.RedisKey()` 返回缓存值实际落在哪个 key 上。不要靠拼字符串反推。
