package urlrefresh

import (
	"errors"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

const (
	JobStatusPending       = "pending"
	JobStatusRunning       = "running"
	JobStatusPendingReview = "pending_review"
	JobStatusApplied       = "applied"
	JobStatusRejected      = "rejected"
	JobStatusFailed        = "failed"
)

var ErrJobLeaseUnavailable = errors.New("url refresh job lease unavailable")
var ErrJobNotFound = errors.New("url refresh job not found")

type CreateJobInput struct {
	ActorID        string
	TenantID       uint64
	KBID           string
	KnowledgeID    string
	SourceURL      string
	ScheduleID     *uint64
	ScheduledFor   *time.Time
	IdempotencyKey string
	Now            time.Time
}

type RunJobInput struct {
	JobID         uint64
	WorkerID      string
	Now           time.Time
	LeaseDuration time.Duration
}

type MarkPendingReviewInput struct {
	JobID          uint64
	WorkerID       string
	Now            time.Time
	FetchedHash    string
	FetchedTitle   string
	FetchedContent string
	DiffSummary    types.JSON
	SSRFCheck      types.JSON
}
