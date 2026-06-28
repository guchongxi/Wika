package search

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupSearchStoreTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&types.Tenant{},
		&types.KnowledgeBase{},
		&types.Knowledge{},
		&types.KnowledgeAccessDaily{},
	))
	require.NoError(t, db.Create(&types.Tenant{ID: 80, Name: "team", SpaceType: types.SpaceTypeTeam}).Error)
	require.NoError(t, db.Create(&types.KnowledgeBase{ID: "kb-team", TenantID: 80, Type: types.KnowledgeBaseTypeDocument}).Error)
	require.NoError(t, db.Create(&types.Knowledge{ID: "k-1", TenantID: 80, KnowledgeBaseID: "kb-team", Title: "知识"}).Error)
	return db
}

func TestGormSearchStoreRecordAccessUpsertsDailyRollup(t *testing.T) {
	db := setupSearchStoreTestDB(t)
	store := NewGormStore(db)
	first := time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC)
	second := time.Date(2026, 6, 29, 18, 0, 0, 0, time.UTC)

	require.NoError(t, store.RecordAccess(context.Background(), []AccessRecord{
		{TenantID: 80, KBID: "kb-team", KnowledgeID: "k-1", AccessedAt: first},
	}))
	require.NoError(t, store.RecordAccess(context.Background(), []AccessRecord{
		{TenantID: 80, KBID: "kb-team", KnowledgeID: "k-1", AccessedAt: second},
	}))

	var access types.KnowledgeAccessDaily
	require.NoError(t, db.First(&access, "tenant_id = ? AND kb_id = ? AND knowledge_id = ?", 80, "kb-team", "k-1").Error)
	if access.AccessCount != 2 {
		t.Fatalf("expected access_count=2, got %d", access.AccessCount)
	}
	if !access.LastAccessedAt.Equal(second) {
		t.Fatalf("expected last_accessed_at=%s, got %s", second, access.LastAccessedAt)
	}
}
