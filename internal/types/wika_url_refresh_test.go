package types

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestWikaURLRefreshTableNames(t *testing.T) {
	if got := (WikaURLRefreshJob{}).TableName(); got != "wika_url_refresh_jobs" {
		t.Fatalf("unexpected job table name: %s", got)
	}
	if got := (WikaURLRefreshSchedule{}).TableName(); got != "wika_url_refresh_schedules" {
		t.Fatalf("unexpected schedule table name: %s", got)
	}
}

func TestWikaURLRefreshTypesCreateCoreFields(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&WikaURLRefreshJob{}, &WikaURLRefreshSchedule{}))

	scheduledFor := time.Now().UTC().Truncate(time.Second)
	job := WikaURLRefreshJob{
		TenantID:       90,
		KBID:           "kb-url",
		KnowledgeID:    "k-url",
		SourceURL:      "https://example.com/doc",
		ScheduledFor:   &scheduledFor,
		IdempotencyKey: "slot-1",
		Status:         "pending",
		DiffSummary:    JSON(`{"changed":true}`),
		SSRFCheck:      JSON(`{"resolver":"fake"}`),
		CreatedBy:      "u-owner",
	}
	require.NoError(t, db.Create(&job).Error)

	schedule := WikaURLRefreshSchedule{
		TenantID:            90,
		KBID:                "kb-url",
		KnowledgeID:         "k-url",
		SourceURL:           "https://example.com/doc",
		Enabled:             true,
		CronExpr:            "0 * * * *",
		NextRunAt:           scheduledFor,
		ConsecutiveFailures: 0,
		CreatedBy:           "u-owner",
	}
	require.NoError(t, db.Create(&schedule).Error)
}
