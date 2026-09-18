package handler

import (
	"time"

	"github.com/boloc/go-frame-server/pkg/alert"
	"github.com/boloc/go-frame-server/pkg/errs"
	"github.com/boloc/go-frame-server/pkg/frame"
	"github.com/boloc/go-frame-server/pkg/frame/rediskey"
	"github.com/boloc/go-frame-server/pkg/frame/reqctx"
	"github.com/boloc/go-frame-server/pkg/frame/webx"
	"github.com/boloc/go-frame-server/pkg/logger"
	"github.com/boloc/go-frame-server/pkg/util/maps"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// CapabilityBodyLimit 回显已被 ContextMiddleware 读入的请求体长度。
// 全局上限由 GinComponent 默认 4MB（middleware.MaxBodyBytes）；超限时进不了本 handler，
// ContextMiddleware 直接 FailWithStatus 413 / code=41300。
//
//	POST /test/body-limit
//	curl -H "X-Demo-Token: x" -H "Content-Type: text/plain" \
//	  --data-binary "hello" localhost:10006/test/body-limit
//	# 预期 code=0，data.bytes=5
//
//	dd if=/dev/zero bs=1024 count=4097 2>/dev/null | \
//	  curl -s -w "\nHTTP %{http_code}\n" -X POST \
//	  -H "X-Demo-Token: x" -H "Content-Type: application/octet-stream" \
//	  --data-binary @- localhost:10006/test/body-limit
//	# 预期 HTTP 413，body.code=41300
func CapabilityBodyLimit(c *gin.Context) {
	rc := reqctx.FromGin(c)
	webx.Success(c, gin.H{
		"bytes": len(rc.RequestBody),
		"hint":  "超过 4MB 时 ContextMiddleware 在进入 handler 前返回 HTTP 413 / code=41300",
	})
}

type slowReq struct {
	Sleep string `form:"sleep"`
}

// CapabilitySlow 演示 RequestTimeout：只取消 c.Request.Context()，handler 必须自己听 ctx.Done()。
// 示例配置默认 request_timeout: 0s（关闭）。把 server.request_timeout 改成 1s 后再 sleep=3s 才能看到超时。
//
//	GET /test/slow?sleep=3s
//	curl -H "X-Demo-Token: x" "localhost:10006/test/slow?sleep=0s"
//	# 预期 code=0
//	# 配置 request_timeout=1s 后：
//	curl -H "X-Demo-Token: x" "localhost:10006/test/slow?sleep=3s"
//	# 预期 code=50004（errs.CodeTimeout）
func CapabilitySlow(c *gin.Context) {
	var req slowReq
	if err := webx.BindQuery(c, &req); err != nil {
		webx.Fail(c, err)
		return
	}

	var sleep time.Duration
	if req.Sleep != "" {
		d, err := time.ParseDuration(req.Sleep)
		if err != nil {
			webx.Fail(c, errs.InvalidParams("sleep 必须是 Go duration，例如 3s、500ms"))
			return
		}
		sleep = d
	}

	ctx := c.Request.Context()
	if sleep <= 0 {
		deadline, ok := ctx.Deadline()
		webx.Success(c, gin.H{
			"slept":            "0s",
			"ctx_has_deadline": ok,
			"ctx_deadline":     deadline,
			"hint":             "把 server.request_timeout 设为 1s 后再请求 ?sleep=3s，会走 ctx.Done() 返回 code=50004",
		})
		return
	}

	timer := time.NewTimer(sleep)
	defer timer.Stop()
	select {
	case <-timer.C:
		deadline, ok := ctx.Deadline()
		webx.Success(c, gin.H{
			"slept":            sleep.String(),
			"ctx_has_deadline": ok,
			"ctx_deadline":     deadline,
		})
	case <-ctx.Done():
		webx.Fail(c, errs.Timeout(ctx.Err()))
	}
}

// CapabilityReqctx 展示 reqctx.FromGin 在 ContextMiddleware 之后能读到的字段。
// RequestID 是单次调用追踪 ID（X-Request-Id），不是幂等键。
//
//	GET /test/reqctx
//	curl -H "X-Demo-Token: x" -H "X-Request-Id: demo-rid" localhost:10006/test/reqctx
//	# 预期 data.request_id=demo-rid，并带回 client_ip
//
//	POST /test/reqctx
//	curl -H "X-Demo-Token: x" -H "Content-Type: text/plain" \
//	  --data-binary "payload" localhost:10006/test/reqctx
//	# 预期 data.request_body=payload
func CapabilityReqctx(c *gin.Context) {
	rc := reqctx.FromGin(c)
	rc.Set("demo_note", "via CustomData")
	note, _ := rc.GetString("demo_note")
	code, codeOK := webx.BizCode(c) // Success/Fail 之前 ok=false
	webx.Success(c, gin.H{
		"request_id":            rc.RequestID,
		"client_ip":             rc.ClientIP,
		"user_agent":            rc.UserAgent,
		"request_body":          string(rc.RequestBody),
		"custom_data.demo_note": note,
		"biz_code_before_write": code,
		"biz_code_ok":           codeOK,
		"hint":                  "RequestID 不是幂等键。AccessLog / HTTPMetrics 在 handler 返回后用 webx.BizCode 读本次业务码。",
	})
}

// validatableDemoReq 演示框架 validate.Validatable：webx.Bind* 在 tag 校验通过后自动调 Validate()。
// 业务 DTO 不要实现这个接口——跨字段规则放 internal/example/validation，由 handler 显式调用。
type validatableDemoReq struct {
	Password string `json:"password" validate:"required,min=6"`
	Confirm  string `json:"confirm" validate:"required"`
}

func (r validatableDemoReq) Validate() error {
	if r.Password != r.Confirm {
		return errs.InvalidParams("两次密码不一致")
	}
	return nil
}

// CapabilityValidatable 演示 Bind 自动调用 Validatable.Validate()。
// 对照 POST /api/orders：那边 DTO 只有 tag，跨字段规则在 validation.ValidateOrderCreate。
//
//	POST /test/validatable
//	curl -H "X-Demo-Token: x" -H "Content-Type: application/json" \
//	  -d '{"password":"secret","confirm":"secret"}' localhost:10006/test/validatable
//	# 预期 code=0
//	curl -H "X-Demo-Token: x" -H "Content-Type: application/json" \
//	  -d '{"password":"secret","confirm":"other"}' localhost:10006/test/validatable
//	# 预期 code=40000，message=两次密码不一致
func CapabilityValidatable(c *gin.Context) {
	var req validatableDemoReq
	if err := webx.BindJSON(c, &req); err != nil {
		webx.Fail(c, err)
		return
	}
	webx.Success(c, gin.H{"ok": true})
}

type mapsDemoItem struct {
	ID    int
	Title string
}

// CapabilityMaps 演示 pkg/util/maps：MapBuilder、SliceToMap、StructSliceToMap、ExtractField、GetOrDefault。
// 下拉选项请用 pkg/util/options（见 GET /api/products/options），不要用 maps 再手写一份。
//
//	GET /test/maps
//	curl -H "X-Demo-Token: x" localhost:10006/test/maps
func CapabilityMaps(c *gin.Context) {
	items := []mapsDemoItem{{ID: 1, Title: "alpha"}, {ID: 2, Title: "beta"}}
	lookup := maps.NewMapBuilder[int, string]().
		Put(1, "alpha").
		Put(2, "beta").
		Build()

	webx.Success(c, gin.H{
		"builder": lookup,
		"by_id": maps.StructSliceToMap(items, func(item mapsDemoItem) int {
			return item.ID
		}),
		"titles": maps.ExtractField(items, func(item mapsDemoItem) string {
			return item.Title
		}),
		"from_slice": maps.SliceToMap(items,
			func(item mapsDemoItem) int { return item.ID },
			func(item mapsDemoItem) string { return item.Title },
		),
		"missing": maps.GetOrDefault(lookup, 99, "unknown"),
	})
}

// CapabilityLogger 演示 logger 字段化用法；stdout 是否 JSON 由 logs.stdout_json → WithLoggerStdoutJSON 决定。
//
//	GET /test/logger
//	curl -H "X-Demo-Token: x" localhost:10006/test/logger
//	# 预期 code=0；日志里有一条 example: structured log demo，带 request_id/route
func CapabilityLogger(c *gin.Context) {
	rc := reqctx.FromGin(c)
	logger.Info("example: structured log demo",
		zap.String("request_id", rc.RequestID),
		zap.String("route", c.FullPath()),
		zap.String("hint", "容器采集 stdout 时把 logs.stdout_json 设为 true"),
	)
	webx.Success(c, gin.H{
		"hint": "看一条带 request_id/route 的 info 日志；JSON 控制台见 logs.stdout_json 与 bootstrap/logger.go",
	})
}

// CapabilityAlertDropped 读取 alert.Dropped()，演示如何把丢弃计数接进日志或指标。
//
//	GET /test/alert-dropped
//	curl -H "X-Demo-Token: x" localhost:10006/test/alert-dropped
//	# 预期 code=0，data.max_in_flight=64
func CapabilityAlertDropped(c *gin.Context) {
	webx.Success(c, gin.H{
		"dropped":       alert.Dropped(),
		"max_in_flight": alert.MaxInFlight,
		"hint":          "Dropped 是 Hook 并发打满 MaxInFlight 后丢弃的累计数。本进程 Hook 见 cmd/example/main.go 的 alert.SetHook。",
	})
}

// CapabilityRedisKey 用 rediskey.App 拼 key，然后 SET 进 Redis，方便在客户端里对照。
//
//	GET /test/redis-key
//	curl -H "X-Demo-Token: x" localhost:10006/test/redis-key
func CapabilityRedisKey(c *gin.Context) {
	key := rediskey.App("product", "123", "stock")
	if err := frame.GetRedisCmdable().Set(c.Request.Context(), key, "demo", 5*time.Minute).Err(); err != nil {
		webx.Fail(c, errs.Database(err))
		return
	}
	webx.Success(c, gin.H{
		"namespace": rediskey.Namespace(),
		"key":       key,
		"value":     "demo",
		"ttl":       "5m",
	})
}
