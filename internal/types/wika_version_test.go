package types

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestWikaKnowledgeVersionTableName(t *testing.T) {
	if got := (WikaKnowledgeVersion{}).TableName(); got != "wika_knowledge_versions" {
		t.Fatalf("unexpected table name: %s", got)
	}
}

func TestWikaKnowledgeVersionNumberIsUniquePerKnowledge(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&Tenant{},
		&KnowledgeBase{},
		&Knowledge{},
		&WikaKnowledgeVersion{},
	))
	require.NoError(t, db.Create(&Tenant{ID: 80, Name: "team", SpaceType: SpaceTypeTeam}).Error)
	require.NoError(t, db.Create(&KnowledgeBase{ID: "kb-team", TenantID: 80, Type: KnowledgeBaseTypeDocument}).Error)
	require.NoError(t, db.Create(&Knowledge{ID: "k-1", TenantID: 80, KnowledgeBaseID: "kb-team", Title: "手册", Type: KnowledgeTypeManual}).Error)

	first := WikaKnowledgeVersion{
		KnowledgeID:  "k-1",
		TenantID:     80,
		KBID:         "kb-team",
		VersionNo:    1,
		Title:        "手册",
		Content:      "旧内容",
		ContentHash:  "hash-1",
		ChangeReason: "manual_update",
		CreatedBy:    "u-owner",
		CreatedAt:    time.Now(),
	}
	require.NoError(t, db.Create(&first).Error)

	duplicate := first
	duplicate.ID = 0
	if err := db.Create(&duplicate).Error; err == nil {
		t.Fatal("expected duplicate knowledge version number to fail")
	}
}
