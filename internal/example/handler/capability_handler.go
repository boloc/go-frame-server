package handler

import (
	"errors"
	"time"

	"github.com/boloc/go-frame-server/internal/example/bizerr"
	"github.com/boloc/go-frame-server/pkg/errs"
	"github.com/boloc/go-frame-server/pkg/frame"
	"github.com/boloc/go-frame-server/pkg/frame/webx"
	"github.com/boloc/go-frame-server/pkg/util"
	"github.com/gin-gonic/gin"
)

// 这一组函数演示框架的响应/错误能力，路由挂在 /test 下，不对应真实业务。

// CapabilitySuccess 演示最基本的成功响应。
// GET /test/success
func CapabilitySuccess(c *gin.Context) {
	webx.Success(c, gin.H{"hello": "world"})
}

// CapabilityNotFound 演示 webx.Fail：HTTP 200，业务码 40400 放在 body.code。
// GET /test/not-found
func CapabilityNotFound(c *gin.Context) {
	webx.Fail(c, errs.NotFound("演示用：这个资源不存在"))
}

// CapabilityNotFoundWithRealStatus 演示 webx.FailWithStatus：响应为真实 HTTP 404。
// GET /test/not-found-with-status
func CapabilityNotFoundWithRealStatus(c *gin.Context) {
	webx.FailWithStatus(c, errs.NotFound("演示用：这个资源不存在"))
}

// CapabilityUnauthorized 演示框架内置错误码 CodeUnauthorized。
// GET /test/unauthorized
func CapabilityUnauthorized(c *gin.Context) {
	webx.Fail(c, errs.Unauthorized(""))
}

// CapabilityDatabaseError 演示 errs.Database：对外统一文案，原始错误只进日志。
// GET /test/database-error
func CapabilityDatabaseError(c *gin.Context) {
	webx.Fail(c, errs.Database(errors.New("dial tcp 127.0.0.1:3306: connect: connection refused")))
}

// CapabilityBusinessError 演示业务自定义错误码 CodeOrderAlreadyPaid（已登记默认文案和 409）。
// GET /test/business-error
func CapabilityBusinessError(c *gin.Context) {
	webx.Fail(c, errs.New(bizerr.CodeOrderAlreadyPaid, ""))
}

// validationDemoReq 只在这个演示接口里使用，用来展示"字段格式校验失败"时响应里的 fields 详情。
type validationDemoReq struct {
	ID uint `form:"id" validate:"required"`
}

// CapabilityValidationError 演示字段格式校验失败：不传 id 或传 id=0 都会触发
// validate:"required"，响应体的 fields 字段会带上具体是哪个字段、为什么不满足。
// GET /test/validation-error
func CapabilityValidationError(c *gin.Context) {
	var req validationDemoReq
	if err := webx.BindQuery(c, &req); err != nil {
		webx.Fail(c, err)
		return
	}
	webx.Success(c, req)
}

// CapabilityPanic 演示 panic 会被 gin.Recovery 兜住，进程不退出，响应是默认 500。
// GET /test/panic
func CapabilityPanic(c *gin.Context) {
	panic("演示用：故意触发一次 panic")
}

// CapabilityErrorSource 演示 errs.Error.Caller 指向真正调用 errs 包的那一行。
// GET /test/error-source
func CapabilityErrorSource(c *gin.Context) {
	err := simulateRepositoryFailure()
	e := errs.From(err)
	webx.Success(c, gin.H{
		"code":    e.Code,
		"message": e.Message,
		"caller":  e.Caller,
		"hint":    "caller 精确指向 capability_handler.go 里 simulateQueryFailure 调用 errs.Database 的那一行，即使中间经过了两层普通函数调用",
	})
}

func simulateRepositoryFailure() error {
	return simulateQueryFailure() // 多包一层普通函数调用，验证 Caller 不会被这层影响
}

func simulateQueryFailure() error {
	return errs.Database(errors.New("connection reset by peer")) // Caller 应该精确指向这一行
}

// timeLayoutWithZone 比 time.DateTime 多带时区偏移和名称，用于本接口对照展示，方便
// 一眼看出两个时间字符串是不是差了一个固定的时区偏移。
const timeLayoutWithZone = time.DateTime + " -0700 MST"

// CapabilityTimezone 让 time.Now() 真的经过 default_db 这个 MySQL 连接写一次、读一次，
// 展示 server.timezone（项目时区）跟这个连接的 loc（默认 UTC，见 util.BuildMysqlDSN）
// 分别对同一个瞬间的解读——不写入任何表，用 SELECT ? 让 MySQL 把参数原样回显，避免
// 污染业务数据。
// GET /test/timezone
func CapabilityTimezone(c *gin.Context) {
	ctx := c.Request.Context()
	now := time.Now()

	var roundTrip time.Time
	if err := frame.DefaultDB().WithContext(ctx).Raw("SELECT ?", now).Scan(&roundTrip).Error; err != nil {
		webx.Fail(c, errs.Database(err))
		return
	}

	webx.Success(c, gin.H{
		"process_timezone":         time.Local.String(),
		"go_time_now":              now.Format(timeLayoutWithZone),
		"mysql_round_trip_raw":     roundTrip.Format(timeLayoutWithZone),
		"mysql_round_trip_display": util.FormatLocal(roundTrip),
		"hint": "go_time_now 是项目时区（server.timezone）下的当前时间；" +
			"mysql_round_trip_raw 是同一个瞬间经过 default_db 连接写入再读出后，按那个连接自己的 loc 解释出来的值；" +
			"mysql_round_trip_display 是再转换回项目时区后应该展示给用户的值——它应该跟 go_time_now 一致（忽略毫秒级误差），" +
			"而 mysql_round_trip_raw 只有在 loc 恰好等于 server.timezone 时才会跟 go_time_now 长得一样",
	})
}
