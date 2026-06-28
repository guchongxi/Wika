package urlrefresh

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupURLRefreshStoreTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&types.Tenant{},
		&types.KnowledgeBase{},
		&types.Knowledge{},
		&types.WikaURLRefreshJob{},
		&types.WikaURLRefreshSchedule{},
	))
	require.NoError(t, db.Create(&types.Tenant{ID: 90, Name: "team", SpaceType: types.SpaceTypeTeam}).Error)
	require.NoError(t, db.Create(&types.KnowledgeBase{ID: "kb-url", TenantID: 90, Type: types.KnowledgeBaseTypeDocument}).Error)
	require.NoError(t, db.Create(&types.Knowledge{ID: "k-url", TenantID: 90, KnowledgeBaseID: "kb-url", Title: "旧标题", Type: types.KnowledgeTypeManual}).Error)
	return db
}

func TestGormStoreCreateAndAcquireJobOnlyOnce(t *testing.T) {
	db := setupURLRefreshStoreTestDB(t)
	store := NewGormStore(db)
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)

	job, err := store.CreateJob(context.Background(), CreateJobInput{
		ActorID:     "u-owner",
		TenantID:    90,
		KBID:        "kb-url",
		KnowledgeID: "k-url",
		SourceURL:   "https://example.com/doc",
		Now:         now,
	})
	require.NoError(t, err)
	if job.Status != JobStatusPending || job.CreatedBy != "u-owner" {
		t.Fatalf("unexpected created job: %+v", job)
	}

	acquired, err := store.AcquireJob(context.Background(), job.ID, "worker-1", now, time.Minute)
	require.NoError(t, err)
	if acquired.Status != JobStatusRunning || acquired.LockedBy != "worker-1" || acquired.Attempts != 1 {
		t.Fatalf("unexpected acquired job: %+v", acquired)
	}
	_, err = store.AcquireJob(context.Background(), job.ID, "worker-2", now, time.Minute)
	if err != ErrJobLeaseUnavailable {
		t.Fatalf("expected lease unavailable, got %v", err)
	}
}
