package evalschedule

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestGormStoreCreateScheduleReturnsConflictForDuplicateEnabledSchedule(t *testing.T) {
	db := setupEvalScheduleTestDB(t)
	store := NewGormStore(db)
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	nextRunAt := now.Add(time.Hour)
	input := CreateScheduleInput{
		ActorID:   "u-owner",
		TenantID:  90,
		KBID:      "kb-eval",
		DatasetID: 11,
		CronExpr:  "0 * * * *",
		Enabled:   true,
		Now:       now,
	}

	if _, err := store.CreateSchedule(context.Background(), input, nextRunAt); err != nil {
		t.Fatalf("create first schedule: %v", err)
	}
	_, err := store.CreateSchedule(context.Background(), input, nextRunAt.Add(time.Hour))
	if !errors.Is(err, ErrScheduleConflict) {
		t.Fatalf("expected ErrScheduleConflict, got %v", err)
	}
}

func TestGormStoreCreateSchedulePersistsDisabledSchedule(t *testing.T) {
	db := setupEvalScheduleTestDB(t)
	store := NewGormStore(db)
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)

	schedule, err := store.CreateSchedule(context.Background(), CreateScheduleInput{
		ActorID:   "u-owner",
		TenantID:  90,
		KBID:      "kb-eval",
		DatasetID: 11,
		CronExpr:  "0 * * * *",
		Enabled:   false,
		Now:       now,
	}, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("create disabled schedule: %v", err)
	}
	if schedule.Enabled {
		t.Fatalf("expected disabled schedule, got enabled")
	}
}

func TestGormStoreUpdateScheduleReturnsConflictWhenEnablingDuplicateSchedule(t *testing.T) {
	db := setupEvalScheduleTestDB(t)
	store := NewGormStore(db)
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	nextRunAt := now.Add(time.Hour)
	input := CreateScheduleInput{
		ActorID:   "u-owner",
		TenantID:  90,
		KBID:      "kb-eval",
		DatasetID: 11,
		CronExpr:  "0 * * * *",
		Enabled:   true,
		Now:       now,
	}
	if _, err := store.CreateSchedule(context.Background(), input, nextRunAt); err != nil {
		t.Fatalf("create enabled schedule: %v", err)
	}
	input.Enabled = false
	disabled, err := store.CreateSchedule(context.Background(), input, nextRunAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("create disabled schedule: %v", err)
	}

	_, err = store.UpdateSchedule(context.Background(), UpdateScheduleInput{
		ActorID:    "u-owner",
		ScheduleID: disabled.ID,
		CronExpr:   "0 * * * *",
		Enabled:    true,
		Now:        now,
	}, nextRunAt.Add(2*time.Hour))
	if !errors.Is(err, ErrScheduleConflict) {
		t.Fatalf("expected ErrScheduleConflict, got %v", err)
	}
}
