package repository

import (
	"context"
	"os"
	"testing"

	"github.com/boloc/go-frame-server/internal/model"
	"github.com/boloc/go-frame-server/pkg/errs"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// newIntegrationProductRepository 连一次真实 MySQL（GFS_TEST_MYSQL_DSN），未设置时跳过——
// UpdateStatus 的这两个场景都依赖 RowsAffected 的真实数据库行为，没法用 fake *gorm.DB 模拟。
func newIntegrationProductRepository(t *testing.T) (*ProductRepository, *gorm.DB) {
	t.Helper()
	dsn := os.Getenv("GFS_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("GFS_TEST_MYSQL_DSN 未设置，跳过需要真实 MySQL 的集成测试")
	}

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := db.AutoMigrate(&model.Product{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}

	// 直接构造 ProductRepository，绕开依赖全局默认实例注册表的 NewProductRepository，
	// 让这个测试不需要碰 components 包的全局状态。
	repo := &ProductRepository{
		master: func() *gorm.DB { return db },
		slave:  func() *gorm.DB { return db },
	}
	return repo, db
}

// TestProductRepositoryUpdateStatusReturnsNotFoundForMissingID 验证更新一个不存在的
// id 会返回 NotFound，而不是静默成功。
func TestProductRepositoryUpdateStatusReturnsNotFoundForMissingID(t *testing.T) {
	repo, _ := newIntegrationProductRepository(t)

	const missingID = 999999999
	err := repo.UpdateStatus(context.Background(), missingID, model.ProductStatusUnlisted)
	if !errs.Is(err, errs.CodeNotFound) {
		t.Fatalf("UpdateStatus(不存在的 id) 的 error = %v, want errs.CodeNotFound", err)
	}
}

// TestProductRepositoryUpdateStatusIsIdempotentWhenStatusUnchanged 验证设置成跟当前值
// 相同的状态是合法的幂等调用，不应该被误判成 NotFound。
func TestProductRepositoryUpdateStatusIsIdempotentWhenStatusUnchanged(t *testing.T) {
	repo, db := newIntegrationProductRepository(t)

	product := &model.Product{Title: "idempotent-status-test", Status: model.ProductStatusListed}
	if err := db.Create(product).Error; err != nil {
		t.Fatalf("create fixture product: %v", err)
	}
	defer db.Unscoped().Delete(product)

	err := repo.UpdateStatus(context.Background(), product.ID, model.ProductStatusListed)
	if err != nil {
		t.Fatalf("UpdateStatus(相同状态) 的 error = %v, want nil（应该是幂等的合法调用）", err)
	}
}
