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
var ErrInvalidJobState = errors.New("url refresh job state conflict")
var ErrInvalidReviewDecision = errors.New("invalid url refresh review decision")
var ErrFeatureDisabled = errors.New("wika url refresh feature disabled")
var ErrInvalidSchedule = errors.New("invalid url refresh schedule")
var ErrScheduleNotFound = errors.New("url refresh schedule not found")
var ErrScheduleLeaseUnavailable = errors.New("url refresh schedule lease unavailable")
var ErrUnsafeSourceURL = errors.New("unsafe source url")

const (
	ReviewDecisionApply  = "apply"
	ReviewDecisionReject = "reject"
)

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

type ListInput struct {
	ActorID     string
	TenantID    uint64
	KBID        string
	KnowledgeID string
	Status      string
	Limit       int
	Offset      int
}

type ListResult struct {
	Jobs      []*types.WikaURLRefreshJob      `json:"jobs"`
	Schedules []*types.WikaURLRefreshSchedule `json:"schedules"`
}

type CreateOrUpdateScheduleInput struct {
	ActorID     string
	TenantID    uint64
	KBID        string
	KnowledgeID string
	SourceURL   string
	CronExpr    string
	Enabled     bool
	Now         time.Time
}

type UpdateScheduleInput struct {
	ActorID    string
	ScheduleID uint64
	CronExpr   string
	Enabled    bool
	Now        time.Time
}

type DisableScheduleInput struct {
	ActorID    string
	ScheduleID uint64
	Now        time.Time
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

type ReviewJobInput struct {
	ActorID  string
	JobID    uint64
	Decision string
	Comment  string
	Now      time.Time
}

type ReviewJobResult struct {
	JobID     uint64 `json:"job_id"`
	Status    string `json:"status"`
	VersionID uint64 `json:"version_id,omitempty"`
}

type MarkReviewedInput struct {
	JobID      uint64
	Status     string
	ActorID    string
	Comment    string
	ReviewedAt time.Time
}

type MarkScheduleFailedInput struct {
	ScheduleID  uint64
	Now         time.Time
	FailureCode string
}
