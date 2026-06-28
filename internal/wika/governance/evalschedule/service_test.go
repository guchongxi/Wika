package evalschedule

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	wikaeval "github.com/Tencent/WeKnora/internal/wika/evaluation"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type fakeEvalRunner struct {
	inputs []wikaeval.RunInput
	err    error
}

func (r *fakeEvalRunner) RunEvaluation(_ context.Context, input wikaeval.RunInput) (*types.WikaEvalRun, error) {
	r.inputs = append(r.inputs, input)
	if r.err != nil {
		return nil, r.err
	}
	return &types.WikaEvalRun{ID: uint64(len(r.inputs)), TenantID: input.TenantID, KBID: input.KBID, DatasetID: input.DatasetID, ScheduleID: input.ScheduleID, ScheduledFor: input.ScheduledFor}, nil
}

type fakeEvalScheduleGate struct {
	enabled bool
}

func (g fakeEvalScheduleGate) GetBool(ctx context.Context, key string, envName string, def bool) bool {
	return g.enabled
}

type reentrantEvalRunner struct {
	svc    *Service
	now    time.Time
	inputs []wikaeval.RunInput
}

func (r *reentrantEvalRunner) RunEvaluation(ctx context.Context, input wikaeval.RunInput) (*types.WikaEvalRun, error) {
	r.inputs = append(r.inputs, input)
	if len(r.inputs) == 1 {
		if _, err := r.svc.RunDueSchedules(ctx, r.now); err != nil {
			return nil, err
		}
	}
	return &types.WikaEvalRun{ID: uint64(len(r.inputs)), TenantID: input.TenantID, KBID: input.KBID, DatasetID: input.DatasetID, ScheduleID: input.ScheduleID, ScheduledFor: input.ScheduledFor}, nil
}

func setupEvalScheduleTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&types.Tenant{},
		&types.KnowledgeBase{},
		&types.WikaEvalDataset{},
		&types.WikaEvalSchedule{},
		&types.WikaEvalRun{},
	))
	require.NoError(t, db.Create(&types.Tenant{ID: 90, Name: "team", SpaceType: types.SpaceTypeTeam}).Error)
	require.NoError(t, db.Create(&types.KnowledgeBase{ID: "kb-eval", TenantID: 90, Type: types.KnowledgeBaseTypeDocument}).Error)
	require.NoError(t, db.Create(&types.WikaEvalDataset{ID: 11, TenantID: 90, KBID: "kb-eval", Name: "黄金 QA", CreatedBy: "u-owner"}).Error)
	return db
}

func TestServiceCreateScheduleRejectsCronBelowMinimum(t *testing.T) {
	db := setupEvalScheduleTestDB(t)
	svc := NewService(NewGormStore(db), &fakeEvalRunner{}, WithFeatureGate(fakeEvalScheduleGate{enabled: true}))
	now := time.Date(2026, 6, 29, 12, 15, 0, 0, time.UTC)

	_, err := svc.CreateSchedule(context.Background(), CreateScheduleInput{
		ActorID:   "u-owner",
		TenantID:  90,
		KBID:      "kb-eval",
		DatasetID: 11,
		CronExpr:  "*/30 * * * *",
		Enabled:   true,
		Now:       now,
	})
	if err != ErrInvalidSchedule {
		t.Fatalf("expected ErrInvalidSchedule, got %v", err)
	}
}

func TestServiceRunDueSchedulesCreatesOneRunPerSlot(t *testing.T) {
	db := setupEvalScheduleTestDB(t)
	runner := &fakeEvalRunner{}
	svc := NewService(NewGormStore(db), runner, WithFeatureGate(fakeEvalScheduleGate{enabled: true}))
	now := time.Date(2026, 6, 29, 12, 15, 0, 0, time.UTC)
	schedule, err := svc.CreateSchedule(context.Background(), CreateScheduleInput{
		ActorID:   "u-owner",
		TenantID:  90,
		KBID:      "kb-eval",
		DatasetID: 11,
		CronExpr:  "0 * * * *",
		Enabled:   true,
		Now:       now,
	})
	require.NoError(t, err)
	dueAt := now.Add(-time.Minute)
	require.NoError(t, db.Model(&types.WikaEvalSchedule{}).Where("id = ?", schedule.ID).Update("next_run_at", dueAt).Error)

	runs, err := svc.RunDueSchedules(context.Background(), now)
	require.NoError(t, err)
	if len(runs) != 1 || len(runner.inputs) != 1 {
		t.Fatalf("expected one run, runs=%+v inputs=%+v", runs, runner.inputs)
	}
	if runner.inputs[0].ScheduleID == nil ||
		*runner.inputs[0].ScheduleID != schedule.ID ||
		runner.inputs[0].ScheduledFor == nil ||
		!runner.inputs[0].ScheduledFor.Equal(dueAt) {
		t.Fatalf("scheduled run input did not carry slot: %+v", runner.inputs[0])
	}

	runs, err = svc.RunDueSchedules(context.Background(), now)
	require.NoError(t, err)
	if len(runs) != 0 || len(runner.inputs) != 1 {
		t.Fatalf("expected second scan to be idempotent, runs=%+v inputs=%+v", runs, runner.inputs)
	}
}

func TestServiceUpdateScheduleResetsNextRunAndFailures(t *testing.T) {
	db := setupEvalScheduleTestDB(t)
	svc := NewService(NewGormStore(db), &fakeEvalRunner{}, WithFeatureGate(fakeEvalScheduleGate{enabled: true}))
	now := time.Date(2026, 6, 29, 12, 15, 0, 0, time.UTC)
	schedule, err := svc.CreateSchedule(context.Background(), CreateScheduleInput{
		ActorID:   "u-owner",
		TenantID:  90,
		KBID:      "kb-eval",
		DatasetID: 11,
		CronExpr:  "0 * * * *",
		Enabled:   true,
		Now:       now,
	})
	require.NoError(t, err)
	require.NoError(t, db.Model(&types.WikaEvalSchedule{}).
		Where("id = ?", schedule.ID).
		Updates(map[string]any{"consecutive_failures": 2, "last_failure_code": "runner_failed"}).Error)

	updated, err := svc.UpdateSchedule(context.Background(), UpdateScheduleInput{
		ActorID:    "u-owner",
		ScheduleID: schedule.ID,
		CronExpr:   "0 3 * * *",
		Enabled:    false,
		Now:        now.Add(time.Hour),
	})
	require.NoError(t, err)
	if updated.Enabled {
		t.Fatal("expected schedule to be disabled by update payload")
	}
	if updated.CronExpr != "0 3 * * *" || updated.ConsecutiveFailures != 0 || updated.LastFailureCode != "" {
		t.Fatalf("unexpected updated schedule: %+v", updated)
	}
}

func TestServiceDisableScheduleStopsFutureRuns(t *testing.T) {
	db := setupEvalScheduleTestDB(t)
	runner := &fakeEvalRunner{}
	svc := NewService(NewGormStore(db), runner, WithFeatureGate(fakeEvalScheduleGate{enabled: true}))
	now := time.Date(2026, 6, 29, 12, 15, 0, 0, time.UTC)
	schedule, err := svc.CreateSchedule(context.Background(), CreateScheduleInput{
		ActorID:   "u-owner",
		TenantID:  90,
		KBID:      "kb-eval",
		DatasetID: 11,
		CronExpr:  "0 * * * *",
		Enabled:   true,
		Now:       now,
	})
	require.NoError(t, err)

	disabled, err := svc.DisableSchedule(context.Background(), DisableScheduleInput{
		ActorID:    "u-owner",
		ScheduleID: schedule.ID,
		Now:        now.Add(time.Minute),
	})
	require.NoError(t, err)
	if disabled.Enabled {
		t.Fatalf("expected disabled schedule, got %+v", disabled)
	}
	require.NoError(t, db.Model(&types.WikaEvalSchedule{}).Where("id = ?", schedule.ID).Update("next_run_at", now.Add(-time.Minute)).Error)
	runs, err := svc.RunDueSchedules(context.Background(), now)
	require.NoError(t, err)
	if len(runs) != 0 || len(runner.inputs) != 0 {
		t.Fatalf("disabled schedule should not run, runs=%+v inputs=%+v", runs, runner.inputs)
	}
}

func TestServiceRunDueSchedulesDisablesAfterThirdFailure(t *testing.T) {
	db := setupEvalScheduleTestDB(t)
	runner := &fakeEvalRunner{err: wikaeval.ErrEvaluationStoreNotConfigured}
	svc := NewService(NewGormStore(db), runner, WithFeatureGate(fakeEvalScheduleGate{enabled: true}))
	now := time.Date(2026, 6, 29, 12, 15, 0, 0, time.UTC)
	schedule, err := svc.CreateSchedule(context.Background(), CreateScheduleInput{
		ActorID:   "u-owner",
		TenantID:  90,
		KBID:      "kb-eval",
		DatasetID: 11,
		CronExpr:  "0 * * * *",
		Enabled:   true,
		Now:       now,
	})
	require.NoError(t, err)
	require.NoError(t, db.Model(&types.WikaEvalSchedule{}).
		Where("id = ?", schedule.ID).
		Updates(map[string]any{
			"next_run_at":          now.Add(-time.Minute),
			"consecutive_failures": 2,
		}).Error)

	_, err = svc.RunDueSchedules(context.Background(), now)
	require.Error(t, err)

	var updated types.WikaEvalSchedule
	require.NoError(t, db.First(&updated, "id = ?", schedule.ID).Error)
	if updated.Enabled || updated.ConsecutiveFailures != 3 || updated.LastFailureCode != "eval_run_failed" {
		t.Fatalf("expected third failure to disable schedule, got %+v", updated)
	}
}

func TestServiceRunDueSchedulesUsesLeaseBeforeCreatingRun(t *testing.T) {
	db := setupEvalScheduleTestDB(t)
	runner := &reentrantEvalRunner{now: time.Date(2026, 6, 29, 12, 15, 0, 0, time.UTC)}
	svc := NewService(NewGormStore(db), runner, WithFeatureGate(fakeEvalScheduleGate{enabled: true}))
	runner.svc = svc
	schedule, err := svc.CreateSchedule(context.Background(), CreateScheduleInput{
		ActorID:   "u-owner",
		TenantID:  90,
		KBID:      "kb-eval",
		DatasetID: 11,
		CronExpr:  "0 * * * *",
		Enabled:   true,
		Now:       runner.now,
	})
	require.NoError(t, err)
	dueAt := runner.now.Add(-time.Minute)
	require.NoError(t, db.Model(&types.WikaEvalSchedule{}).Where("id = ?", schedule.ID).Update("next_run_at", dueAt).Error)

	runs, err := svc.RunDueSchedules(context.Background(), runner.now)
	require.NoError(t, err)
	if len(runs) != 1 || len(runner.inputs) != 1 {
		t.Fatalf("expected lease to prevent duplicate scheduled run, runs=%+v inputs=%+v", runs, runner.inputs)
	}
}
