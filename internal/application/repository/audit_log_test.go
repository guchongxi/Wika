package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/Tencent/WeKnora/internal/database"
	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAuditLogRepositoryCreateUsesGormTransactionFromContext(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&types.AuditLog{}); err != nil {
		t.Fatalf("migrate audit log: %v", err)
	}
	repo := NewAuditLogRepository(db)
	rollbackErr := errors.New("rollback audit transaction")
	err = db.Transaction(func(tx *gorm.DB) error {
		ctx := database.WithGormTransaction(context.Background(), tx)
		if err := repo.Create(ctx, &types.AuditLog{TenantID: 80, Action: types.AuditActionWikaOrgShareCreated}); err != nil {
			return err
		}
		return rollbackErr
	})
	if !errors.Is(err, rollbackErr) {
		t.Fatalf("expected rollback error, got %v", err)
	}
	var count int64
	if err := db.Model(&types.AuditLog{}).Count(&count).Error; err != nil {
		t.Fatalf("count audit logs: %v", err)
	}
	if count != 0 {
		t.Fatalf("audit log must be rolled back with context transaction, got count=%d", count)
	}
}
