package urlrefresh

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/wika/governance/urlrefresh/safefetch"
	"github.com/Tencent/WeKnora/internal/wika/governance/version"
)

type Fetcher interface {
	Fetch(ctx context.Context, raw string) (*safefetch.FetchResult, error)
}

type SourceURLValidator interface {
	ValidateURL(ctx context.Context, raw string) error
}

type KnowledgeUpdater interface {
	UpdateManualKnowledge(ctx context.Context, knowledgeID string, payload *types.ManualKnowledgePayload) (*types.Knowledge, error)
}

type VersionRecorder interface {
	RecordVersion(ctx context.Context, input version.RecordVersionInput) (*types.WikaKnowledgeVersion, error)
}

type AuditLogger interface {
	Log(ctx context.Context, entry *types.AuditLog) error
}

type FeatureGate interface {
	GetBool(ctx context.Context, key string, envName string, def bool) bool
}

type Service struct {
	store     Store
	fetcher   Fetcher
	knowledge KnowledgeUpdater
	versions  VersionRecorder
	audit     AuditLogger
	flags     FeatureGate
}

type ServiceOption func(*Service)

const urlRefreshFeatureFlagKey = "wika.governance.url_refresh.enabled"
const minScheduleInterval = time.Hour
const defaultScheduleWorkerID = "wika-url-refresh-scheduler"

func WithKnowledgeUpdater(knowledge KnowledgeUpdater) ServiceOption {
	return func(s *Service) {
		s.knowledge = knowledge
	}
}

func WithVersionRecorder(recorder VersionRecorder) ServiceOption {
	return func(s *Service) {
		s.versions = recorder
	}
}

func WithAuditLogger(audit AuditLogger) ServiceOption {
	return func(s *Service) {
		s.audit = audit
	}
}

func WithFeatureGate(flags FeatureGate) ServiceOption {
	return func(s *Service) {
		s.flags = flags
	}
}

func NewService(store Store, fetcher Fetcher, opts ...ServiceOption) *Service {
	svc := &Service{store: store, fetcher: fetcher}
	for _, opt := range opts {
		opt(svc)
	}
	return svc
}

func (s *Service) CreateJob(ctx context.Context, input CreateJobInput) (*types.WikaURLRefreshJob, error) {
	if !s.featureEnabled(ctx) {
		return nil, ErrFeatureDisabled
	}
	if s.store == nil {
		return nil, nil
	}
	if err := s.validateSourceURL(ctx, input.SourceURL); err != nil {
		return nil, err
	}
	return s.store.CreateJob(ctx, input)
}

func (s *Service) List(ctx context.Context, input ListInput) (*ListResult, error) {
	if !s.featureEnabled(ctx) {
		return nil, ErrFeatureDisabled
	}
	if s.store == nil {
		return &ListResult{}, nil
	}
	return s.store.List(ctx, input)
}

func (s *Service) RunJob(ctx context.Context, input RunJobInput) error {
	if !s.featureEnabled(ctx) {
		return ErrFeatureDisabled
	}
	if s.store == nil {
		return nil
	}
	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}
	workerID := strings.TrimSpace(input.WorkerID)
	if workerID == "" {
		workerID = "wika-url-refresh-worker"
	}
	job, err := s.store.AcquireJob(ctx, input.JobID, workerID, now, input.LeaseDuration)
	if err != nil {
		return err
	}
	if s.fetcher == nil {
		err := fmt.Errorf("url refresh fetcher unavailable")
		return s.failJobAndSchedule(ctx, job, workerID, now, "fetcher_unavailable", err)
	}
	result, err := s.fetcher.Fetch(ctx, job.SourceURL)
	if err != nil {
		return s.failJobAndSchedule(ctx, job, workerID, now, "fetch_failed", err)
	}
	ssrfCheck, _ := json.Marshal(map[string]any{
		"content_type": result.ContentType,
		"final_url":    result.FinalURL,
		"size_bytes":   result.SizeBytes,
	})
	if err := s.store.MarkPendingReview(ctx, MarkPendingReviewInput{
		JobID:          job.ID,
		WorkerID:       workerID,
		Now:            now,
		FetchedHash:    hashFetchedContent(result.Text),
		FetchedContent: result.Text,
		DiffSummary:    types.JSON([]byte(`{}`)),
		SSRFCheck:      types.JSON(ssrfCheck),
	}); err != nil {
		return err
	}
	if job.ScheduleID != nil {
		if err := s.store.MarkScheduleSucceeded(ctx, *job.ScheduleID, now); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) CreateOrUpdateSchedule(ctx context.Context, input CreateOrUpdateScheduleInput) (*types.WikaURLRefreshSchedule, error) {
	if !s.featureEnabled(ctx) {
		return nil, ErrFeatureDisabled
	}
	if s.store == nil {
		return nil, nil
	}
	if err := s.validateSourceURL(ctx, input.SourceURL); err != nil {
		return nil, err
	}
	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}
	schedule, nextRunAt, err := parseSchedule(input.CronExpr, now)
	if err != nil {
		return nil, err
	}
	if interval := schedule.Next(nextRunAt).Sub(nextRunAt); interval < minScheduleInterval {
		return nil, ErrInvalidSchedule
	}
	input.Now = now
	return s.store.CreateOrUpdateSchedule(ctx, input, nextRunAt)
}

func (s *Service) RunDueSchedules(ctx context.Context, now time.Time) ([]*types.WikaURLRefreshJob, error) {
	if !s.featureEnabled(ctx) {
		return nil, ErrFeatureDisabled
	}
	if s.store == nil {
		return nil, nil
	}
	if now.IsZero() {
		now = time.Now()
	}
	schedules, err := s.store.ListDueSchedules(ctx, now, 100)
	if err != nil {
		return nil, err
	}
	jobs := make([]*types.WikaURLRefreshJob, 0, len(schedules))
	for _, item := range schedules {
		if item == nil {
			continue
		}
		locked, err := s.store.AcquireSchedule(ctx, item.ID, defaultScheduleWorkerID, now, time.Minute)
		if err != nil {
			if err == ErrScheduleLeaseUnavailable {
				continue
			}
			return nil, err
		}
		item = locked
		scheduledFor := item.NextRunAt
		existing, err := s.store.FindJobByScheduleSlot(ctx, item.ID, scheduledFor)
		if err == nil {
			nextRunAt, nextErr := nextScheduleRun(item.CronExpr, scheduledFor)
			if nextErr != nil {
				return nil, nextErr
			}
			if err := s.store.MarkScheduleTriggered(ctx, item.ID, existing.ID, nextRunAt, now); err != nil {
				return nil, err
			}
			continue
		}
		if err != ErrJobNotFound {
			return nil, err
		}
		scheduleID := item.ID
		job, err := s.store.CreateJob(ctx, CreateJobInput{
			ActorID:      item.CreatedBy,
			TenantID:     item.TenantID,
			KBID:         item.KBID,
			KnowledgeID:  item.KnowledgeID,
			SourceURL:    item.SourceURL,
			ScheduleID:   &scheduleID,
			ScheduledFor: &scheduledFor,
			Now:          now,
		})
		if err != nil {
			return nil, err
		}
		nextRunAt, err := nextScheduleRun(item.CronExpr, scheduledFor)
		if err != nil {
			return nil, err
		}
		if err := s.store.MarkScheduleTriggered(ctx, item.ID, job.ID, nextRunAt, now); err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

func (s *Service) RunRunnableJobs(ctx context.Context, now time.Time) ([]*types.WikaURLRefreshJob, error) {
	if !s.featureEnabled(ctx) {
		return nil, ErrFeatureDisabled
	}
	if s.store == nil {
		return nil, nil
	}
	if now.IsZero() {
		now = time.Now()
	}
	return s.store.ListRunnableJobs(ctx, now, 100)
}

func (s *Service) validateSourceURL(ctx context.Context, raw string) error {
	validator, ok := s.fetcher.(SourceURLValidator)
	if !ok || validator == nil {
		return nil
	}
	if err := validator.ValidateURL(ctx, raw); err != nil {
		return fmt.Errorf("%w: %v", ErrUnsafeSourceURL, err)
	}
	return nil
}

func (s *Service) UpdateSchedule(ctx context.Context, input UpdateScheduleInput) (*types.WikaURLRefreshSchedule, error) {
	if !s.featureEnabled(ctx) {
		return nil, ErrFeatureDisabled
	}
	if s.store == nil {
		return nil, ErrScheduleNotFound
	}
	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}
	schedule, nextRunAt, err := parseSchedule(input.CronExpr, now)
	if err != nil {
		return nil, err
	}
	if interval := schedule.Next(nextRunAt).Sub(nextRunAt); interval < minScheduleInterval {
		return nil, ErrInvalidSchedule
	}
	input.Now = now
	return s.store.UpdateSchedule(ctx, input, nextRunAt)
}

func (s *Service) DisableSchedule(ctx context.Context, input DisableScheduleInput) (*types.WikaURLRefreshSchedule, error) {
	if !s.featureEnabled(ctx) {
		return nil, ErrFeatureDisabled
	}
	if s.store == nil {
		return nil, ErrScheduleNotFound
	}
	if input.Now.IsZero() {
		input.Now = time.Now()
	}
	return s.store.DisableSchedule(ctx, input)
}

func (s *Service) ReviewJob(ctx context.Context, input ReviewJobInput) (*ReviewJobResult, error) {
	if !s.featureEnabled(ctx) {
		return nil, ErrFeatureDisabled
	}
	if s.store == nil {
		return nil, ErrJobNotFound
	}
	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}
	switch input.Decision {
	case ReviewDecisionApply:
		return s.applyJob(ctx, input, now)
	case ReviewDecisionReject:
		job, err := s.store.GetJob(ctx, input.JobID)
		if err != nil {
			return nil, err
		}
		if job.Status != JobStatusPendingReview {
			return nil, ErrInvalidJobState
		}
		if err := s.store.MarkReviewed(ctx, MarkReviewedInput{
			JobID:      input.JobID,
			Status:     JobStatusRejected,
			ActorID:    input.ActorID,
			Comment:    input.Comment,
			ReviewedAt: now,
		}); err != nil {
			return nil, err
		}
		if err := s.logReviewed(ctx, input.ActorID, job.TenantID, input.JobID); err != nil {
			return nil, err
		}
		return &ReviewJobResult{JobID: input.JobID, Status: JobStatusRejected}, nil
	default:
		return nil, ErrInvalidReviewDecision
	}
}

func (s *Service) applyJob(ctx context.Context, input ReviewJobInput, now time.Time) (*ReviewJobResult, error) {
	if s.knowledge == nil {
		return nil, fmt.Errorf("knowledge updater unavailable")
	}
	if s.versions == nil {
		return nil, fmt.Errorf("version recorder unavailable")
	}
	job, err := s.store.GetJob(ctx, input.JobID)
	if err != nil {
		return nil, err
	}
	if job.Status != JobStatusPendingReview {
		return nil, ErrInvalidJobState
	}
	title := strings.TrimSpace(job.FetchedTitle)
	if title == "" {
		title = job.KnowledgeID
	}
	if _, err := s.knowledge.UpdateManualKnowledge(ctx, job.KnowledgeID, &types.ManualKnowledgePayload{
		Title:             title,
		Content:           job.FetchedContent,
		Status:            types.ManualKnowledgeStatusPublish,
		Channel:           types.ChannelWeb,
		SkipVersionRecord: true,
	}); err != nil {
		return nil, err
	}
	recorded, err := s.versions.RecordVersion(ctx, version.RecordVersionInput{
		KnowledgeID:  job.KnowledgeID,
		TenantID:     job.TenantID,
		KBID:         job.KBID,
		Title:        title,
		Content:      job.FetchedContent,
		Status:       types.ManualKnowledgeStatusPublish,
		ChangeReason: "url_refresh_apply",
		ActorID:      input.ActorID,
		Now:          now,
		ContentHash:  job.FetchedHash,
	})
	if err != nil {
		return nil, err
	}
	if err := s.store.MarkReviewed(ctx, MarkReviewedInput{
		JobID:      job.ID,
		Status:     JobStatusApplied,
		ActorID:    input.ActorID,
		Comment:    input.Comment,
		ReviewedAt: now,
	}); err != nil {
		return nil, err
	}
	if err := s.logReviewed(ctx, input.ActorID, job.TenantID, job.ID); err != nil {
		return nil, err
	}
	var versionID uint64
	if recorded != nil {
		versionID = recorded.ID
	}
	return &ReviewJobResult{JobID: job.ID, Status: JobStatusApplied, VersionID: versionID}, nil
}

func (s *Service) logReviewed(ctx context.Context, actorID string, tenantID uint64, jobID uint64) error {
	if s.audit == nil {
		return nil
	}
	return s.audit.Log(ctx, &types.AuditLog{
		TenantID:    tenantID,
		ActorUserID: strings.TrimSpace(actorID),
		Action:      types.AuditActionWikaURLRefreshReviewed,
		TargetType:  "wika_url_refresh_job",
		TargetID:    strconv.FormatUint(jobID, 10),
	})
}

func (s *Service) failJobAndSchedule(ctx context.Context, job *types.WikaURLRefreshJob, workerID string, now time.Time, failureCode string, cause error) error {
	errMsg := ""
	if cause != nil {
		errMsg = cause.Error()
	}
	if err := s.store.FailJob(ctx, job.ID, workerID, now, failureCode, errMsg); err != nil {
		return err
	}
	if job.ScheduleID == nil {
		return cause
	}
	schedule, err := s.store.MarkScheduleFailed(ctx, MarkScheduleFailedInput{
		ScheduleID:  *job.ScheduleID,
		Now:         now,
		FailureCode: failureCode,
	})
	if err != nil {
		return err
	}
	action := types.AuditActionWikaURLRefreshScheduleUpdated
	if !schedule.Enabled {
		action = types.AuditActionWikaURLRefreshScheduleDisabled
	}
	if err := s.logScheduleFailure(ctx, action, schedule); err != nil {
		return err
	}
	return cause
}

func (s *Service) logScheduleFailure(ctx context.Context, action types.AuditAction, schedule *types.WikaURLRefreshSchedule) error {
	if s.audit == nil || schedule == nil {
		return nil
	}
	details, _ := json.Marshal(map[string]any{
		"consecutive_failures": schedule.ConsecutiveFailures,
		"failure_code":         schedule.LastFailureCode,
		"enabled":              schedule.Enabled,
	})
	return s.audit.Log(ctx, &types.AuditLog{
		TenantID:    schedule.TenantID,
		ActorUserID: strings.TrimSpace(schedule.CreatedBy),
		Action:      action,
		TargetType:  "wika_url_refresh_schedule",
		TargetID:    strconv.FormatUint(schedule.ID, 10),
		Details:     types.JSON(details),
	})
}

func (s *Service) featureEnabled(ctx context.Context) bool {
	if s.flags == nil {
		return false
	}
	return s.flags.GetBool(ctx, urlRefreshFeatureFlagKey, "", false)
}

func hashFetchedContent(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func parseSchedule(expr string, now time.Time) (cron.Schedule, time.Time, error) {
	parsed, err := cron.ParseStandard(strings.TrimSpace(expr))
	if err != nil {
		return nil, time.Time{}, ErrInvalidSchedule
	}
	nextRunAt := parsed.Next(now)
	if nextRunAt.IsZero() {
		return nil, time.Time{}, ErrInvalidSchedule
	}
	return parsed, nextRunAt, nil
}

func nextScheduleRun(expr string, after time.Time) (time.Time, error) {
	parsed, nextRunAt, err := parseSchedule(expr, after)
	if err != nil {
		return time.Time{}, err
	}
	if parsed.Next(nextRunAt).Sub(nextRunAt) < minScheduleInterval {
		return time.Time{}, ErrInvalidSchedule
	}
	return nextRunAt, nil
}

func truncateError(msg string) string {
	msg = strings.TrimSpace(msg)
	if len([]rune(msg)) <= 512 {
		return msg
	}
	runes := []rune(msg)
	return string(runes[:512])
}

func urlRefreshScheduleFailureBackoff(failures int, scheduleID uint64) time.Duration {
	jitter := time.Duration(scheduleID%300) * time.Second
	if failures <= 1 {
		return time.Hour + jitter
	}
	return 6*time.Hour + jitter
}
