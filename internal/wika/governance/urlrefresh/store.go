package urlrefresh

import (
	"context"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/gorm"
)

type Store interface {
	CreateJob(ctx context.Context, input CreateJobInput) (*types.WikaURLRefreshJob, error)
	GetJob(ctx context.Context, jobID uint64) (*types.WikaURLRefreshJob, error)
	AcquireJob(ctx context.Context, jobID uint64, workerID string, now time.Time, lease time.Duration) (*types.WikaURLRefreshJob, error)
	MarkPendingReview(ctx context.Context, input MarkPendingReviewInput) error
	MarkReviewed(ctx context.Context, input MarkReviewedInput) error
	FailJob(ctx context.Context, jobID uint64, workerID string, now time.Time, failureCode string, errMsg string) error
	CreateOrUpdateSchedule(ctx context.Context, input CreateOrUpdateScheduleInput, nextRunAt time.Time) (*types.WikaURLRefreshSchedule, error)
	ListDueSchedules(ctx context.Context, now time.Time, limit int) ([]*types.WikaURLRefreshSchedule, error)
	FindJobByScheduleSlot(ctx context.Context, scheduleID uint64, scheduledFor time.Time) (*types.WikaURLRefreshJob, error)
	MarkScheduleTriggered(ctx context.Context, scheduleID uint64, jobID uint64, nextRunAt time.Time, now time.Time) error
	MarkScheduleSucceeded(ctx context.Context, scheduleID uint64, now time.Time) error
	MarkScheduleFailed(ctx context.Context, input MarkScheduleFailedInput) (*types.WikaURLRefreshSchedule, error)
	UpdateSchedule(ctx context.Context, input UpdateScheduleInput, nextRunAt time.Time) (*types.WikaURLRefreshSchedule, error)
	DisableSchedule(ctx context.Context, input DisableScheduleInput) (*types.WikaURLRefreshSchedule, error)
}

type GormStore struct {
	db *gorm.DB
}

func NewGormStore(db *gorm.DB) *GormStore {
	return &GormStore{db: db}
}

func (s *GormStore) CreateJob(ctx context.Context, input CreateJobInput) (*types.WikaURLRefreshJob, error) {
	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}
	job := &types.WikaURLRefreshJob{
		ScheduleID:     input.ScheduleID,
		KnowledgeID:    input.KnowledgeID,
		TenantID:       input.TenantID,
		KBID:           input.KBID,
		SourceURL:      input.SourceURL,
		ScheduledFor:   input.ScheduledFor,
		IdempotencyKey: input.IdempotencyKey,
		Status:         JobStatusPending,
		DiffSummary:    types.JSON([]byte(`{}`)),
		SSRFCheck:      types.JSON([]byte(`{}`)),
		CreatedBy:      input.ActorID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.db.WithContext(ctx).Create(job).Error; err != nil {
		return nil, err
	}
	return job, nil
}

func (s *GormStore) GetJob(ctx context.Context, jobID uint64) (*types.WikaURLRefreshJob, error) {
	var job types.WikaURLRefreshJob
	err := s.db.WithContext(ctx).First(&job, "id = ?", jobID).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrJobNotFound
		}
		return nil, err
	}
	return &job, nil
}

func (s *GormStore) AcquireJob(ctx context.Context, jobID uint64, workerID string, now time.Time, lease time.Duration) (*types.WikaURLRefreshJob, error) {
	if lease <= 0 {
		lease = time.Minute
	}
	lockedUntil := now.Add(lease)
	result := s.db.WithContext(ctx).Model(&types.WikaURLRefreshJob{}).
		Where("id = ?", jobID).
		Where("status IN ?", []string{JobStatusPending, JobStatusRunning}).
		Where("locked_until IS NULL OR locked_until < ?", now).
		Updates(map[string]any{
			"status":       JobStatusRunning,
			"locked_by":    workerID,
			"locked_until": lockedUntil,
			"attempts":     gorm.Expr("attempts + 1"),
			"started_at":   gorm.Expr("COALESCE(started_at, ?)", now),
			"updated_at":   now,
		})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, ErrJobLeaseUnavailable
	}
	var job types.WikaURLRefreshJob
	if err := s.db.WithContext(ctx).First(&job, "id = ?", jobID).Error; err != nil {
		return nil, err
	}
	return &job, nil
}

func (s *GormStore) MarkPendingReview(ctx context.Context, input MarkPendingReviewInput) error {
	result := s.db.WithContext(ctx).Model(&types.WikaURLRefreshJob{}).
		Where("id = ? AND locked_by = ?", input.JobID, input.WorkerID).
		Updates(map[string]any{
			"status":          JobStatusPendingReview,
			"fetched_hash":    input.FetchedHash,
			"fetched_title":   input.FetchedTitle,
			"fetched_content": input.FetchedContent,
			"diff_summary":    jsonOrDefault(input.DiffSummary),
			"ssrf_check":      jsonOrDefault(input.SSRFCheck),
			"completed_at":    input.Now,
			"locked_by":       "",
			"locked_until":    nil,
			"updated_at":      input.Now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrJobLeaseUnavailable
	}
	return nil
}

func (s *GormStore) MarkReviewed(ctx context.Context, input MarkReviewedInput) error {
	result := s.db.WithContext(ctx).Model(&types.WikaURLRefreshJob{}).
		Where("id = ? AND status = ?", input.JobID, JobStatusPendingReview).
		Updates(map[string]any{
			"status":         input.Status,
			"reviewed_by":    input.ActorID,
			"reviewed_at":    input.ReviewedAt,
			"review_comment": input.Comment,
			"updated_at":     input.ReviewedAt,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrInvalidJobState
	}
	return nil
}

func (s *GormStore) FailJob(ctx context.Context, jobID uint64, workerID string, now time.Time, failureCode string, errMsg string) error {
	result := s.db.WithContext(ctx).Model(&types.WikaURLRefreshJob{}).
		Where("id = ? AND locked_by = ?", jobID, workerID).
		Updates(map[string]any{
			"status":       JobStatusFailed,
			"failure_code": failureCode,
			"error_msg":    truncateError(errMsg),
			"completed_at": now,
			"locked_by":    "",
			"locked_until": nil,
			"updated_at":   now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrJobLeaseUnavailable
	}
	return nil
}

func (s *GormStore) CreateOrUpdateSchedule(ctx context.Context, input CreateOrUpdateScheduleInput, nextRunAt time.Time) (*types.WikaURLRefreshSchedule, error) {
	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}
	var schedule types.WikaURLRefreshSchedule
	err := s.db.WithContext(ctx).
		Where("knowledge_id = ? AND source_url = ?", input.KnowledgeID, input.SourceURL).
		First(&schedule).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}
	if err == gorm.ErrRecordNotFound {
		schedule = types.WikaURLRefreshSchedule{
			TenantID:            input.TenantID,
			KBID:                input.KBID,
			KnowledgeID:         input.KnowledgeID,
			SourceURL:           input.SourceURL,
			Enabled:             input.Enabled,
			CronExpr:            input.CronExpr,
			NextRunAt:           nextRunAt,
			ConsecutiveFailures: 0,
			CreatedBy:           input.ActorID,
			CreatedAt:           now,
			UpdatedAt:           now,
		}
		if err := s.db.WithContext(ctx).Create(&schedule).Error; err != nil {
			return nil, err
		}
		return &schedule, nil
	}
	if err := s.db.WithContext(ctx).Model(&schedule).Updates(map[string]any{
		"enabled":              input.Enabled,
		"cron_expr":            input.CronExpr,
		"next_run_at":          nextRunAt,
		"consecutive_failures": 0,
		"last_failure_code":    "",
		"updated_at":           now,
	}).Error; err != nil {
		return nil, err
	}
	return &schedule, nil
}

func (s *GormStore) ListDueSchedules(ctx context.Context, now time.Time, limit int) ([]*types.WikaURLRefreshSchedule, error) {
	if limit <= 0 {
		limit = 100
	}
	var schedules []*types.WikaURLRefreshSchedule
	err := s.db.WithContext(ctx).
		Where("enabled = ?", true).
		Where("next_run_at <= ?", now).
		Order("next_run_at ASC").
		Limit(limit).
		Find(&schedules).Error
	return schedules, err
}

func (s *GormStore) FindJobByScheduleSlot(ctx context.Context, scheduleID uint64, scheduledFor time.Time) (*types.WikaURLRefreshJob, error) {
	var job types.WikaURLRefreshJob
	err := s.db.WithContext(ctx).
		Where("schedule_id = ? AND scheduled_for = ?", scheduleID, scheduledFor).
		First(&job).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrJobNotFound
		}
		return nil, err
	}
	return &job, nil
}

func (s *GormStore) MarkScheduleTriggered(ctx context.Context, scheduleID uint64, jobID uint64, nextRunAt time.Time, now time.Time) error {
	return s.db.WithContext(ctx).Model(&types.WikaURLRefreshSchedule{}).
		Where("id = ?", scheduleID).
		Updates(map[string]any{
			"last_job_id":  jobID,
			"next_run_at":  nextRunAt,
			"locked_by":    "",
			"locked_until": nil,
			"updated_at":   now,
		}).Error
}

func (s *GormStore) MarkScheduleSucceeded(ctx context.Context, scheduleID uint64, now time.Time) error {
	result := s.db.WithContext(ctx).Model(&types.WikaURLRefreshSchedule{}).
		Where("id = ?", scheduleID).
		Updates(map[string]any{
			"consecutive_failures": 0,
			"last_failure_code":    "",
			"updated_at":           now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrScheduleNotFound
	}
	return nil
}

func (s *GormStore) MarkScheduleFailed(ctx context.Context, input MarkScheduleFailedInput) (*types.WikaURLRefreshSchedule, error) {
	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}
	var schedule types.WikaURLRefreshSchedule
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&schedule, "id = ?", input.ScheduleID).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return ErrScheduleNotFound
			}
			return err
		}
		failures := schedule.ConsecutiveFailures + 1
		updates := map[string]any{
			"consecutive_failures": failures,
			"last_failure_code":    input.FailureCode,
			"locked_by":            "",
			"locked_until":         nil,
			"updated_at":           now,
		}
		if failures >= 3 {
			updates["enabled"] = false
		} else {
			updates["next_run_at"] = now.Add(urlRefreshScheduleFailureBackoff(failures, input.ScheduleID))
		}
		if err := tx.Model(&types.WikaURLRefreshSchedule{}).
			Where("id = ?", input.ScheduleID).
			Updates(updates).Error; err != nil {
			return err
		}
		return tx.First(&schedule, "id = ?", input.ScheduleID).Error
	})
	if err != nil {
		return nil, err
	}
	return &schedule, nil
}

func (s *GormStore) UpdateSchedule(ctx context.Context, input UpdateScheduleInput, nextRunAt time.Time) (*types.WikaURLRefreshSchedule, error) {
	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}
	var schedule types.WikaURLRefreshSchedule
	err := s.db.WithContext(ctx).First(&schedule, "id = ?", input.ScheduleID).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrScheduleNotFound
		}
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&schedule).Updates(map[string]any{
		"enabled":              input.Enabled,
		"cron_expr":            input.CronExpr,
		"next_run_at":          nextRunAt,
		"consecutive_failures": 0,
		"last_failure_code":    "",
		"updated_at":           now,
	}).Error; err != nil {
		return nil, err
	}
	return &schedule, nil
}

func (s *GormStore) DisableSchedule(ctx context.Context, input DisableScheduleInput) (*types.WikaURLRefreshSchedule, error) {
	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}
	var schedule types.WikaURLRefreshSchedule
	err := s.db.WithContext(ctx).First(&schedule, "id = ?", input.ScheduleID).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrScheduleNotFound
		}
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&schedule).Updates(map[string]any{
		"enabled":      false,
		"updated_at":   now,
		"locked_by":    "",
		"locked_until": nil,
	}).Error; err != nil {
		return nil, err
	}
	return &schedule, nil
}

func jsonOrDefault(value types.JSON) types.JSON {
	if len(value) == 0 {
		return types.JSON([]byte(`{}`))
	}
	return value
}
