package conflict

import (
	"context"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

type Store interface {
	AcquireCheck(ctx context.Context, checkID uint64, workerID string, now time.Time, lease time.Duration) (*types.WikaConflictCheck, error)
	SaveConflictItems(ctx context.Context, check *types.WikaConflictCheck, items []Candidate) error
	CompleteCheck(ctx context.Context, checkID uint64, workerID string, now time.Time) error
	FailCheck(ctx context.Context, checkID uint64, workerID string, now time.Time, errMsg string) error
}

type CandidateGenerator interface {
	GenerateCandidates(ctx context.Context, input GenerateInput) ([]Candidate, error)
}

// Service 执行冲突检测 worker 流程。
type Service struct {
	store     Store
	generator CandidateGenerator
}

func NewService(store *GormStore, generator CandidateGenerator) *Service {
	return &Service{store: store, generator: generator}
}

type RunCheckInput struct {
	CheckID       uint64
	WorkerID      string
	Now           time.Time
	LeaseDuration time.Duration
}

func (s *Service) RunCheck(ctx context.Context, input RunCheckInput) error {
	if s.store == nil {
		return nil
	}
	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}
	workerID := strings.TrimSpace(input.WorkerID)
	if workerID == "" {
		workerID = "wika-conflict-worker"
	}
	check, err := s.store.AcquireCheck(ctx, input.CheckID, workerID, now, input.LeaseDuration)
	if err != nil {
		return err
	}
	candidates, err := s.generateCandidates(ctx, check)
	if err != nil {
		_ = s.store.FailCheck(ctx, check.ID, workerID, now, truncateError(err.Error()))
		return err
	}
	if err := s.store.SaveConflictItems(ctx, check, candidates); err != nil {
		_ = s.store.FailCheck(ctx, check.ID, workerID, now, truncateError(err.Error()))
		return err
	}
	return s.store.CompleteCheck(ctx, check.ID, workerID, now)
}

func (s *Service) generateCandidates(ctx context.Context, check *types.WikaConflictCheck) ([]Candidate, error) {
	if s.generator == nil || check == nil {
		return nil, nil
	}
	return s.generator.GenerateCandidates(ctx, GenerateInput{
		CheckID:  check.ID,
		TenantID: check.TenantID,
		KBID:     check.KBID,
		Trigger:  check.Trigger,
	})
}

func truncateError(msg string) string {
	msg = strings.TrimSpace(msg)
	if len([]rune(msg)) <= 512 {
		return msg
	}
	runes := []rune(msg)
	return string(runes[:512])
}
