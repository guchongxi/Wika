package evalschedule

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

type Store interface {
	CreateSchedule(ctx context.Context, input CreateScheduleInput, nextRunAt time.Time) (*types.WikaEvalSchedule, error)
	List(ctx context.Context, input ListInput) (*ListResult, error)
	ListDueSchedules(ctx context.Context, now time.Time, limit int) ([]*types.WikaEvalSchedule, error)
	AcquireSchedule(ctx context.Context, scheduleID uint64, workerID string, now time.Time, lease time.Duration) (*types.WikaEvalSchedule, error)
	MarkScheduleTriggered(ctx context.Context, scheduleID uint64, runID uint64, nextRunAt time.Time, now time.Time) error
	MarkScheduleFailed(ctx context.Context, scheduleID uint64, now time.Time, failureCode string) (*types.WikaEvalSchedule, error)
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
	if err := s.db.WithContext(ctx).
		Select("TenantID", "KBID", "DatasetID", "Enabled", "CronExpr", "NextRunAt", "ConsecutiveFailures", "LastFailureCode", "LockedBy", "CreatedBy", "CreatedAt", "UpdatedAt").
		Create(schedule).Error; err != nil {
		if isScheduleUniqueConflict(err) {
			return nil, ErrScheduleConflict
		}
		return nil, err
	}
	return schedule, nil
}

func (s *GormStore) List(ctx context.Context, input ListInput) (*ListResult, error) {
	limit := input.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	offset := input.Offset
	if offset < 0 {
		offset = 0
	}
	query := s.db.WithContext(ctx).
		Model(&types.WikaEvalSchedule{}).
		Where("tenant_id = ? AND kb_id = ?", input.TenantID, input.KBID)
	if input.Enabled != nil {
		query = query.Where("enabled = ?", *input.Enabled)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var schedules []*types.WikaEvalSchedule
	if err := query.
		Order("enabled DESC, updated_at DESC, id DESC").
		Limit(limit).
		Offset(offset).
		Find(&schedules).Error; err != nil {
		return nil, err
	}
	return &ListResult{Schedules: schedules, Total: total}, nil
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

func isScheduleUniqueConflict(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" && pgErr.ConstraintName == "ux_wika_eval_schedules_enabled"
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "wika_eval_schedules") &&
		(strings.Contains(msg, "ux_wika_eval_schedules_enabled") ||
			strings.Contains(msg, "unique constraint failed"))
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

func (s *GormStore) MarkScheduleFailed(ctx context.Context, scheduleID uint64, now time.Time, failureCode string) (*types.WikaEvalSchedule, error) {
	var schedule types.WikaEvalSchedule
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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
			schedule.Enabled = false
		} else {
			nextRunAt := now.Add(evalScheduleFailureBackoff(failures))
			updates["next_run_at"] = nextRunAt
			schedule.NextRunAt = nextRunAt
		}
		if err := tx.Model(&types.WikaEvalSchedule{}).Where("id = ?", scheduleID).Updates(updates).Error; err != nil {
			return err
		}
		schedule.ConsecutiveFailures = failures
		schedule.LastFailureCode = failureCode
		schedule.LockedBy = ""
		schedule.LockedUntil = nil
		schedule.UpdatedAt = now
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &schedule, nil
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
		if isScheduleUniqueConflict(result.Error) {
			return nil, ErrScheduleConflict
		}
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
