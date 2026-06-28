package evalschedule

import (
	"context"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/gorm"
)

type Store interface {
	CreateSchedule(ctx context.Context, input CreateScheduleInput, nextRunAt time.Time) (*types.WikaEvalSchedule, error)
	ListDueSchedules(ctx context.Context, now time.Time, limit int) ([]*types.WikaEvalSchedule, error)
	AcquireSchedule(ctx context.Context, scheduleID uint64, workerID string, now time.Time, lease time.Duration) (*types.WikaEvalSchedule, error)
	MarkScheduleTriggered(ctx context.Context, scheduleID uint64, runID uint64, nextRunAt time.Time, now time.Time) error
	MarkScheduleFailed(ctx context.Context, scheduleID uint64, now time.Time, failureCode string) error
	UpdateSchedule(ctx context.Context, input UpdateScheduleInput, nextRunAt time.Time) (*types.WikaEvalSchedule, error)
	DisableSchedule(ctx context.Context, input DisableScheduleInput) (*types.WikaEvalSchedule, error)
}

type GormStore struct {
	db *gorm.DB
}

func NewGormStore(db *gorm.DB) *GormStore {
	return &GormStore{db: db}
}

func (s *GormStore) CreateSchedule(ctx context.Context, input CreateScheduleInput, nextRunAt time.Time) (*types.WikaEvalSchedule, error) {
	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}
	schedule := &types.WikaEvalSchedule{
		TenantID:            input.TenantID,
		KBID:                input.KBID,
		DatasetID:           input.DatasetID,
		Enabled:             input.Enabled,
		CronExpr:            input.CronExpr,
		NextRunAt:           nextRunAt,
		ConsecutiveFailures: 0,
		CreatedBy:           input.ActorID,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	if err := s.db.WithContext(ctx).Create(schedule).Error; err != nil {
		return nil, err
	}
	return schedule, nil
}

func (s *GormStore) ListDueSchedules(ctx context.Context, now time.Time, limit int) ([]*types.WikaEvalSchedule, error) {
	if limit <= 0 {
		limit = 100
	}
	var schedules []*types.WikaEvalSchedule
	err := s.db.WithContext(ctx).
		Where("enabled = ?", true).
		Where("next_run_at <= ?", now).
		Where("locked_until IS NULL OR locked_until < ?", now).
		Order("next_run_at ASC").
		Limit(limit).
		Find(&schedules).Error
	return schedules, err
}

func (s *GormStore) AcquireSchedule(ctx context.Context, scheduleID uint64, workerID string, now time.Time, lease time.Duration) (*types.WikaEvalSchedule, error) {
	if lease <= 0 {
		lease = time.Minute
	}
	result := s.db.WithContext(ctx).Model(&types.WikaEvalSchedule{}).
		Where("id = ?", scheduleID).
		Where("enabled = ?", true).
		Where("next_run_at <= ?", now).
		Where("locked_until IS NULL OR locked_until < ?", now).
		Updates(map[string]any{
			"locked_by":    workerID,
			"locked_until": now.Add(lease),
			"updated_at":   now,
		})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, ErrScheduleLeaseUnavailable
	}
	var schedule types.WikaEvalSchedule
	if err := s.db.WithContext(ctx).First(&schedule, "id = ?", scheduleID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrScheduleNotFound
		}
		return nil, err
	}
	return &schedule, nil
}

func (s *GormStore) MarkScheduleTriggered(ctx context.Context, scheduleID uint64, runID uint64, nextRunAt time.Time, now time.Time) error {
	return s.db.WithContext(ctx).Model(&types.WikaEvalSchedule{}).
		Where("id = ?", scheduleID).
		Updates(map[string]any{
			"last_run_id":          runID,
			"next_run_at":          nextRunAt,
			"consecutive_failures": 0,
			"last_failure_code":    "",
			"locked_by":            "",
			"locked_until":         nil,
			"updated_at":           now,
		}).Error
}

func (s *GormStore) MarkScheduleFailed(ctx context.Context, scheduleID uint64, now time.Time, failureCode string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var schedule types.WikaEvalSchedule
		if err := tx.First(&schedule, "id = ?", scheduleID).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return ErrScheduleNotFound
			}
			return err
		}
		failures := schedule.ConsecutiveFailures + 1
		updates := map[string]any{
			"consecutive_failures": failures,
			"last_failure_code":    failureCode,
			"locked_by":            "",
			"locked_until":         nil,
			"updated_at":           now,
		}
		if failures >= 3 {
			updates["enabled"] = false
		} else {
			updates["next_run_at"] = now.Add(evalScheduleFailureBackoff(failures))
		}
		return tx.Model(&types.WikaEvalSchedule{}).Where("id = ?", scheduleID).Updates(updates).Error
	})
}

func (s *GormStore) UpdateSchedule(ctx context.Context, input UpdateScheduleInput, nextRunAt time.Time) (*types.WikaEvalSchedule, error) {
	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}
	result := s.db.WithContext(ctx).Model(&types.WikaEvalSchedule{}).
		Where("id = ?", input.ScheduleID).
		Updates(map[string]any{
			"enabled":              input.Enabled,
			"cron_expr":            input.CronExpr,
			"next_run_at":          nextRunAt,
			"consecutive_failures": 0,
			"last_failure_code":    "",
			"updated_at":           now,
		})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, ErrScheduleNotFound
	}
	var schedule types.WikaEvalSchedule
	if err := s.db.WithContext(ctx).First(&schedule, "id = ?", input.ScheduleID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrScheduleNotFound
		}
		return nil, err
	}
	return &schedule, nil
}

func (s *GormStore) DisableSchedule(ctx context.Context, input DisableScheduleInput) (*types.WikaEvalSchedule, error) {
	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}
	result := s.db.WithContext(ctx).Model(&types.WikaEvalSchedule{}).
		Where("id = ?", input.ScheduleID).
		Updates(map[string]any{
			"enabled":      false,
			"locked_by":    "",
			"locked_until": nil,
			"updated_at":   now,
		})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, ErrScheduleNotFound
	}
	var schedule types.WikaEvalSchedule
	if err := s.db.WithContext(ctx).First(&schedule, "id = ?", input.ScheduleID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrScheduleNotFound
		}
		return nil, err
	}
	return &schedule, nil
}
