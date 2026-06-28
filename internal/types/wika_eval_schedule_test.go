package types

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestWikaEvalScheduleTableNameAndCoreFields(t *testing.T) {
	if got := (WikaEvalSchedule{}).TableName(); got != "wika_eval_schedules" {
		t.Fatalf("unexpected eval schedule table name: %s", got)
	}

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&WikaEvalSchedule{}))

	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	schedule := WikaEvalSchedule{
		TenantID:            90,
		KBID:                "kb-team",
		DatasetID:           11,
		Enabled:             true,
		CronExpr:            "0 * * * *",
		NextRunAt:           now,
		ConsecutiveFailures: 0,
		CreatedBy:           "u-owner",
	}
	require.NoError(t, db.Create(&schedule).Error)
	if schedule.ID == 0 {
		t.Fatal("expected schedule id to be generated")
	}
}
