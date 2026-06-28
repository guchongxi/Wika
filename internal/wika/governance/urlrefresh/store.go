package urlrefresh

import (
	"context"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/gorm"
)

type Store interface {
	CreateJob(ctx context.Context, input CreateJobInput) (*types.WikaURLRefreshJob, error)
	AcquireJob(ctx context.Context, jobID uint64, workerID string, now time.Time, lease time.Duration) (*types.WikaURLRefreshJob, error)
	MarkPendingReview(ctx context.Context, input MarkPendingReviewInput) error
	FailJob(ctx context.Context, jobID uint64, workerID string, now time.Time, failureCode string, errMsg string) error
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

func jsonOrDefault(value types.JSON) types.JSON {
	if len(value) == 0 {
		return types.JSON([]byte(`{}`))
	}
	return value
}
