package conflict

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupConflictStoreTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&types.Tenant{},
		&types.KnowledgeBase{},
		&types.Knowledge{},
		&types.WikaConflictCheck{},
		&types.WikaConflictItem{},
	))
	return db
}

func TestGormConflictStoreAcquireCheckOnlyOnce(t *testing.T) {
	db := setupConflictStoreTestDB(t)
	require.NoError(t, db.Create(&types.Tenant{ID: 80, Name: "team", SpaceType: types.SpaceTypeTeam}).Error)
	require.NoError(t, db.Create(&types.KnowledgeBase{ID: "kb-team", TenantID: 80, Type: types.KnowledgeBaseTypeDocument}).Error)
	check := &types.WikaConflictCheck{TenantID: 80, KBID: "kb-team", Trigger: TriggerManual, Status: CheckStatusPending, CreatedBy: "u-owner"}
	require.NoError(t, db.Create(check).Error)

	store := NewGormStore(db)
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	acquired, err := store.AcquireCheck(context.Background(), check.ID, "worker-1", now, time.Minute)
	require.NoError(t, err)
	if acquired.LockedBy != "worker-1" || acquired.Attempts != 1 || acquired.Status != CheckStatusRunning {
		t.Fatalf("unexpected acquired check: %+v", acquired)
	}

	_, err = store.AcquireCheck(context.Background(), check.ID, "worker-2", now, time.Minute)
	if err == nil {
		t.Fatal("expected second worker to fail acquiring locked check")
	}
	if err != ErrCheckLeaseUnavailable {
		t.Fatalf("expected ErrCheckLeaseUnavailable, got %v", err)
	}
}

func TestGormConflictStoreSavesCandidatesAndCompletesCheck(t *testing.T) {
	db := setupConflictStoreTestDB(t)
	require.NoError(t, db.Create(&types.Tenant{ID: 80, Name: "team", SpaceType: types.SpaceTypeTeam}).Error)
	require.NoError(t, db.Create(&types.KnowledgeBase{ID: "kb-team", TenantID: 80, Type: types.KnowledgeBaseTypeDocument}).Error)
	require.NoError(t, db.Create(&types.Knowledge{ID: "k-1", TenantID: 80, KnowledgeBaseID: "kb-team", Title: "A"}).Error)
	require.NoError(t, db.Create(&types.Knowledge{ID: "k-2", TenantID: 80, KnowledgeBaseID: "kb-team", Title: "B"}).Error)
	check := &types.WikaConflictCheck{TenantID: 80, KBID: "kb-team", Trigger: TriggerManual, Status: CheckStatusRunning, LockedBy: "worker-1", CreatedBy: "u-owner"}
	require.NoError(t, db.Create(check).Error)
	store := NewGormStore(db)
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)

	err := store.SaveConflictItems(context.Background(), check, []Candidate{
		{
			SourceKnowledgeID: "k-1",
			TargetKnowledgeID: "k-2",
			ConflictType:      ConflictTypeContradiction,
			ConfidenceScore:   0.91,
			Evidence:          types.JSON([]byte(`{"source_hash":"a","target_hash":"b"}`)),
			AIExplanation:     "两条知识结论相反",
		},
	})
	require.NoError(t, err)
	require.NoError(t, store.CompleteCheck(context.Background(), check.ID, "worker-1", now))

	var item types.WikaConflictItem
	require.NoError(t, db.First(&item, "check_id = ?", check.ID).Error)
	if item.Status != ItemStatusOpen || item.SourceKnowledgeID != "k-1" || item.TargetKnowledgeID != "k-2" {
		t.Fatalf("unexpected conflict item: %+v", item)
	}
	var completed types.WikaConflictCheck
	require.NoError(t, db.First(&completed, "id = ?", check.ID).Error)
	if completed.Status != CheckStatusCompleted || completed.LockedBy != "" || completed.CompletedAt == nil {
		t.Fatalf("unexpected completed check: %+v", completed)
	}
}
