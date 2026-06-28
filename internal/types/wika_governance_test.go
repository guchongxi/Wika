package types

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestWikaP5ConflictTableNames(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{name: "conflict checks", got: (WikaConflictCheck{}).TableName(), want: "wika_conflict_checks"},
		{name: "conflict items", got: (WikaConflictItem{}).TableName(), want: "wika_conflict_items"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Fatalf("expected table name %q, got %q", tc.want, tc.got)
			}
		})
	}
}

func TestWikaConflictOpenItemsAreUnique(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&Tenant{},
		&KnowledgeBase{},
		&Knowledge{},
		&WikaConflictCheck{},
		&WikaConflictItem{},
	))
	require.NoError(t, db.Create(&Tenant{ID: 80, Name: "team", SpaceType: SpaceTypeTeam}).Error)
	require.NoError(t, db.Create(&KnowledgeBase{ID: "kb-team", TenantID: 80, Type: KnowledgeBaseTypeDocument}).Error)
	require.NoError(t, db.Create(&Knowledge{ID: "k-1", TenantID: 80, KnowledgeBaseID: "kb-team", Title: "A"}).Error)
	require.NoError(t, db.Create(&Knowledge{ID: "k-2", TenantID: 80, KnowledgeBaseID: "kb-team", Title: "B"}).Error)
	check := &WikaConflictCheck{TenantID: 80, KBID: "kb-team", Trigger: "manual", Status: "completed", CreatedBy: "u-owner"}
	require.NoError(t, db.Create(check).Error)

	item := WikaConflictItem{
		CheckID:           check.ID,
		TenantID:          80,
		KBID:              "kb-team",
		SourceKnowledgeID: "k-1",
		TargetKnowledgeID: "k-2",
		ConflictType:      "contradiction",
		Status:            "open",
	}
	require.NoError(t, db.Create(&item).Error)

	duplicate := item
	duplicate.ID = 0
	err = db.Create(&duplicate).Error
	if err == nil {
		t.Fatal("expected duplicate open conflict item to violate unique constraint")
	}

	resolved := item
	resolved.ID = 0
	resolved.Status = "resolved"
	require.NoError(t, db.Create(&resolved).Error)
}
