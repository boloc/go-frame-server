package handler

import (
	"sync"
	"time"

	"github.com/boloc/go-frame-server/internal/model"
	"github.com/boloc/go-frame-server/pkg/errs"
	"github.com/boloc/go-frame-server/pkg/frame/components"
	"github.com/boloc/go-frame-server/pkg/frame/reqctx"
	"github.com/boloc/go-frame-server/pkg/frame/webx"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"uuid"
)

var clickhouseDemoOnce sync.Once

const clickhouseDemoCreateTable = `
CREATE TABLE IF NOT EXISTS example_demo_events (
	event_id String,
	name String,
	created_at DateTime
) ENGINE = MergeTree()
ORDER BY (created_at, event_id)
`

// clickhouseDemoReq 可选事件名；不传时用固定演示值。
type clickhouseDemoReq struct {
	Name string `json:"name" validate:"omitempty,max=64"`
}

// ClickHouseEventRoundTrip 写一条事件到 ClickHouse 再按 event_id 查回来。
// clickhouse.enabled=false 或尚未连接时返回业务错误，不 panic。
//
//	POST /test/clickhouse/events
//	curl -H "X-Demo-Token: x" -H "Content-Type: application/json" \
//	  -d '{"name":"demo"}' localhost:10006/test/clickhouse/events
//	# 未启用：code=50003，message 含「ClickHouse 未启用」
//	# 已启用：code=0，data 里 written 与 loaded 同一条 event_id
func ClickHouseEventRoundTrip(c *gin.Context) {
	db, ok := components.TryDefaultClickHouseDB()
	if !ok || db == nil {
		webx.Fail(c, errs.New(errs.CodeDependency, "ClickHouse 未启用（clickhouse.enabled=false 或尚未连接）"))
		return
	}

	var req clickhouseDemoReq
	if len(reqctx.FromGin(c).RequestBody) > 0 {
		if err := webx.BindJSON(c, &req); err != nil {
			webx.Fail(c, err)
			return
		}
	}
	if req.Name == "" {
		req.Name = "demo-event"
	}

	if err := ensureClickHouseDemoTable(db); err != nil {
		webx.Fail(c, errs.Dependency(err))
		return
	}

	ctx := c.Request.Context()
	event := model.ExampleClickHouseEvent{
		EventID:   uuid.New().String(),
		Name:      req.Name,
		CreatedAt: time.Now().UTC().Truncate(time.Second),
	}
	if err := db.WithContext(ctx).Create(&event).Error; err != nil {
		webx.Fail(c, errs.Dependency(err))
		return
	}

	var loaded model.ExampleClickHouseEvent
	if err := db.WithContext(ctx).Where("event_id = ?", event.EventID).Take(&loaded).Error; err != nil {
		webx.Fail(c, errs.Dependency(err))
		return
	}

	webx.Success(c, gin.H{
		"written": event,
		"loaded":  loaded,
		"hint":    "用 components.TryDefaultClickHouseDB，未启用时返回业务错误而不是 panic",
	})
}

func ensureClickHouseDemoTable(db *gorm.DB) error {
	var err error
	clickhouseDemoOnce.Do(func() {
		err = db.Exec(clickhouseDemoCreateTable).Error
	})
	return err
}
