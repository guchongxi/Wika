package types

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestWikaOrgShareTableNameAndCoreFields(t *testing.T) {
	if got := (WikaOrgShare{}).TableName(); got != "wika_org_shares" {
		t.Fatalf("unexpected org share table name: %s", got)
	}

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&WikaOrgShare{}))

	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	share := WikaOrgShare{
		OrgID:          "org-1",
		SourceTenantID: 80,
		SourceKBID:     "kb-source",
		TargetTenantID: 90,
		Mode:           WikaOrgShareModeReference,
		AllowedFields:  JSON([]byte(`["id","title"]`)),
		Status:         WikaOrgShareStatusPending,
		CreatedBy:      "u-owner",
		CreatedAt:      now,
	}
	require.NoError(t, db.Create(&share).Error)
	if share.ID == 0 {
		t.Fatal("expected share id to be generated")
	}
}
