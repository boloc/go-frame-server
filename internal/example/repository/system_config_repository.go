package repository

import (
	"context"
	"errors"
	"time"

	"github.com/boloc/go-frame-server/internal/example/constant"
	"github.com/boloc/go-frame-server/internal/model"
	"github.com/boloc/go-frame-server/pkg/errs"
	"github.com/boloc/go-frame-server/pkg/frame"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SystemConfigRepository 访问 config_db 上的系统配置。读用 slave，写用 master。
type SystemConfigRepository struct {
	master func() *gorm.DB
	slave  func() *gorm.DB
}

// NewSystemConfigRepository 创建 SystemConfigRepository。连接按命名实例延迟获取。
func NewSystemConfigRepository() *SystemConfigRepository {
	return &SystemConfigRepository{
		master: func() *gorm.DB { return frame.MasterDB(constant.MySQLConfigDB) },
		slave:  func() *gorm.DB { return frame.SlaveDB(constant.MySQLConfigDB) },
	}
}

// GetValue 按 key 读取配置，读从库。不存在返回 NotFound。
func (r *SystemConfigRepository) GetValue(ctx context.Context, key string) (string, error) {
	var cfg model.SystemConfig
	err := r.slave().WithContext(ctx).Where("`key` = ?", key).First(&cfg).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", errs.NotFound("配置项 " + key + " 不存在")
	}
	if err != nil {
		return "", errs.Database(err)
	}
	return cfg.Value, nil
}

// SetValue 写入配置（不存在则创建），写主库。
func (r *SystemConfigRepository) SetValue(ctx context.Context, key, value string) error {
	cfg := &model.SystemConfig{Key: key, Value: value, UpdatedAt: time.Now()}
	err := r.master().WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
	}).Create(cfg).Error
	if err != nil {
		return errs.Database(err)
	}
	return nil
}
