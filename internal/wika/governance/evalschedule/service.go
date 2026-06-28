package evalschedule

import (
	"context"
	stderrors "errors"
	"strings"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/Tencent/WeKnora/internal/types"
	wikaeval "github.com/Tencent/WeKnora/internal/wika/evaluation"
)

type EvalRunner interface {
	RunEvaluation(ctx context.Context, input wikaeval.RunInput) (*types.WikaEvalRun, error)
}

type FeatureGate interface {
	GetBool(ctx context.Context, key string, envName string, def bool) bool
}

type Service struct {
	store  Store
	runner EvalRunner
	flags  FeatureGate
}

type ServiceOption func(*Service)

const evalScheduleFeatureFlagKey = "wika.governance.eval_schedule.enabled"
const minScheduleInterval = time.Hour
const defaultScheduleWorkerID = "wika-eval-scheduler"

func WithFeatureGate(flags FeatureGate) ServiceOption {
	return func(s *Service) {
		s.flags = flags
	}
}

func NewService(store Store, runner EvalRunner, opts ...ServiceOption) *Service {
	svc := &Service{store: store, runner: runner}
	for _, opt := range opts {
		opt(svc)
	}
	return svc
}

func (s *Service) CreateSchedule(ctx context.Context, input CreateScheduleInput) (*types.WikaEvalSchedule, error) {
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
	input.CronExpr = strings.TrimSpace(input.CronExpr)
	return s.store.CreateSchedule(ctx, input, nextRunAt)
}

func (s *Service) RunDueSchedules(ctx context.Context, now time.Time) ([]*types.WikaEvalRun, error) {
	if !s.featureEnabled(ctx) {
		return nil, ErrFeatureDisabled
	}
	if s.store == nil {
		return nil, ErrScheduleNotFound
	}
	if s.runner == nil {
		return nil, wikaeval.ErrEvaluationStoreNotConfigured
	}
	if now.IsZero() {
		now = time.Now()
	}
	schedules, err := s.store.ListDueSchedules(ctx, now, 100)
	if err != nil {
		return nil, err
	}
	runs := make([]*types.WikaEvalRun, 0, len(schedules))
	for _, item := range schedules {
		if item == nil {
			continue
		}
		locked, err := s.store.AcquireSchedule(ctx, item.ID, defaultScheduleWorkerID, now, time.Minute)
		if err != nil {
			if stderrors.Is(err, ErrScheduleLeaseUnavailable) {
				continue
			}
			return nil, err
		}
		item = locked
		scheduleID := item.ID
		scheduledFor := item.NextRunAt
		run, err := s.runner.RunEvaluation(ctx, wikaeval.RunInput{
			ActorID:      item.CreatedBy,
			TenantID:     item.TenantID,
			KBID:         item.KBID,
			DatasetID:    item.DatasetID,
			ScheduleID:   &scheduleID,
			ScheduledFor: &scheduledFor,
		})
		if err != nil {
			if markErr := s.store.MarkScheduleFailed(ctx, item.ID, now, "eval_run_failed"); markErr != nil {
				return nil, markErr
			}
			return nil, err
		}
		nextRunAt, err := nextScheduleRun(item.CronExpr, scheduledFor)
		if err != nil {
			return nil, err
		}
		if err := s.store.MarkScheduleTriggered(ctx, item.ID, run.ID, nextRunAt, now); err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, nil
}

func (s *Service) UpdateSchedule(ctx context.Context, input UpdateScheduleInput) (*types.WikaEvalSchedule, error) {
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
	input.CronExpr = strings.TrimSpace(input.CronExpr)
	return s.store.UpdateSchedule(ctx, input, nextRunAt)
}

func (s *Service) DisableSchedule(ctx context.Context, input DisableScheduleInput) (*types.WikaEvalSchedule, error) {
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

func (s *Service) featureEnabled(ctx context.Context) bool {
	if s.flags == nil {
		return false
	}
	return s.flags.GetBool(ctx, evalScheduleFeatureFlagKey, "", false)
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

func evalScheduleFailureBackoff(failures int) time.Duration {
	if failures <= 1 {
		return time.Hour
	}
	return 6 * time.Hour
}
