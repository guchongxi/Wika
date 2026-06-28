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

func TestGormVersionStoreRecordVersionSkipsUnchangedSnapshot(t *testing.T) {
	db := setupVersionStoreTestDB(t)
	store := NewGormStore(db)

	first, err := store.RecordVersion(context.Background(), RecordVersionInput{
		KnowledgeID:  "k-1",
		TenantID:     80,
		KBID:         "kb-team",
		Title:        "手册",
		Content:      "第一版内容",
		Tags:         types.JSON([]byte(`["tag-a"]`)),
		Status:       "publish",
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
		Content:      "第一版内容",
		Tags:         types.JSON([]byte(`["tag-a"]`)),
		Status:       "publish",
		ContentHash:  "hash-1",
		ChangeReason: "restore",
		ActorID:      "u-owner",
	})
	require.NoError(t, err)

	var count int64
	require.NoError(t, db.Model(&types.WikaKnowledgeVersion{}).Where("knowledge_id = ?", "k-1").Count(&count).Error)
	if count != 1 || second.ID != first.ID || second.VersionNo != first.VersionNo {
		t.Fatalf("expected duplicate snapshot to reuse first version, count=%d first=%+v second=%+v", count, first, second)
	}
}

func TestGormVersionStoreListVersionsNewestFirst(t *testing.T) {
	db := setupVersionStoreTestDB(t)
	store := NewGormStore(db)
	for _, content := range []string{"第一版内容", "第二版内容"} {
		_, err := store.RecordVersion(context.Background(), RecordVersionInput{
			KnowledgeID:  "k-1",
			TenantID:     80,
			KBID:         "kb-team",
			Title:        "手册",
			Content:      content,
			ContentHash:  content,
			ChangeReason: "manual_update",
			ActorID:      "u-owner",
		})
		require.NoError(t, err)
	}

	versions, err := store.ListVersions(context.Background(), ListVersionsInput{
		TenantID:    80,
		KnowledgeID: "k-1",
	})
	require.NoError(t, err)
	if len(versions) != 2 || versions[0].VersionNo != 2 || versions[1].VersionNo != 1 {
		t.Fatalf("expected newest first versions, got %+v", versions)
	}
}
