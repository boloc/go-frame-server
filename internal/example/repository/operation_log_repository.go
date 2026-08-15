package repository

import (
	"context"

	"github.com/boloc/go-frame-server/internal/example/constant"
	"github.com/boloc/go-frame-server/internal/model"
	"github.com/boloc/go-frame-server/pkg/errs"
	"github.com/boloc/go-frame-server/pkg/frame"
	"gorm.io/gorm"
)

// OperationLogRepository 访问 log_db 上的审计日志，只连这一个实例。
type OperationLogRepository struct {
	master func() *gorm.DB
}

// NewOperationLogRepository 创建 OperationLogRepository。
func NewOperationLogRepository() *OperationLogRepository {
	return &OperationLogRepository{
		master: func() *gorm.DB { return frame.MasterDB(constant.MySQLLogDB) },
	}
}

// Create 写入一条操作审计日志。
func (r *OperationLogRepository) Create(ctx context.Context, action, targetType string, targetID uint, detail string) error {
	log := &model.OperationLog{Action: action, TargetType: targetType, TargetID: targetID, Detail: detail}
	if err := r.master().WithContext(ctx).Create(log).Error; err != nil {
		return errs.Database(err)
	}
	return nil
}
