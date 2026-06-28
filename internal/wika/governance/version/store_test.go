package version

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupVersionStoreTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&types.Tenant{},
		&types.KnowledgeBase{},
		&types.Knowledge{},
		&types.WikaKnowledgeVersion{},
	))
	require.NoError(t, db.Create(&types.Tenant{ID: 80, Name: "team", SpaceType: types.SpaceTypeTeam}).Error)
	require.NoError(t, db.Create(&types.KnowledgeBase{ID: "kb-team", TenantID: 80, Type: types.KnowledgeBaseTypeDocument}).Error)
	require.NoError(t, db.Create(&types.Knowledge{ID: "k-1", TenantID: 80, KnowledgeBaseID: "kb-team", Title: "手册", Type: types.KnowledgeTypeManual}).Error)
	return db
}

func TestGormVersionStoreRecordVersionIncrementsVersionNo(t *testing.T) {
	db := setupVersionStoreTestDB(t)
	store := NewGormStore(db)

	first, err := store.RecordVersion(context.Background(), RecordVersionInput{
		KnowledgeID:  "k-1",
		TenantID:     80,
		KBID:         "kb-team",
		Title:        "手册",
		Content:      "第一版内容",
		ContentHash:  "hash-1",
		ChangeReason: "manual_update",
		ActorID:      "u-owner",
	})
	require.NoError(t, err)
	second, err := store.RecordVersion(context.Background(), RecordVersionInput{
		KnowledgeID:  "k-1",
		TenantID:     80,
		KBID:         "kb-team",
		Title:        "手册",
		Content:      "第二版内容",
		ContentHash:  "hash-2",
		ChangeReason: "manual_update",
		ActorID:      "u-owner",
	})
	require.NoError(t, err)

	if first.VersionNo != 1 || second.VersionNo != 2 {
		t.Fatalf("unexpected version numbers: first=%d second=%d", first.VersionNo, second.VersionNo)
	}
}
