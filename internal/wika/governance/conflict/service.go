package conflict

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

type Store interface {
	CreateCheck(ctx context.Context, check *types.WikaConflictCheck) (*types.WikaConflictCheck, error)
	ListItems(ctx context.Context, input ListItemsInput) ([]*types.WikaConflictItem, int64, error)
	ResolveItem(ctx context.Context, input ResolveItemInput) (*types.WikaConflictItem, error)
	ListRunnableChecks(ctx context.Context, now time.Time, limit int) ([]*types.WikaConflictCheck, error)
	AcquireCheck(ctx context.Context, checkID uint64, workerID string, now time.Time, lease time.Duration) (*types.WikaConflictCheck, error)
	SaveConflictItems(ctx context.Context, check *types.WikaConflictCheck, items []Candidate) error
	CompleteCheck(ctx context.Context, checkID uint64, workerID string, now time.Time) error
	FailCheck(ctx context.Context, checkID uint64, workerID string, now time.Time, errMsg string) error
}

type CandidateGenerator interface {
	GenerateCandidates(ctx context.Context, input GenerateInput) ([]Candidate, error)
}

type auditLogger interface {
	Log(ctx context.Context, entry *types.AuditLog) error
}

type FeatureGate interface {
	GetBool(ctx context.Context, key string, envName string, def bool) bool
}

// Service 执行冲突检测 worker 流程。
type Service struct {
	store     Store
	generator CandidateGenerator
	audit     auditLogger
	flags     FeatureGate
}

func NewService(store *GormStore, generator CandidateGenerator, audit interfaces.AuditLogService, flags interfaces.SystemSettingService) *Service {
	return &Service{store: store, generator: generator, audit: audit, flags: flags}
}

type noopCandidateGenerator struct{}

func NewNoopCandidateGenerator() CandidateGenerator {
	return noopCandidateGenerator{}
}

func (noopCandidateGenerator) GenerateCandidates(ctx context.Context, input GenerateInput) ([]Candidate, error) {
	return nil, nil
}

func (s *Service) CreateCheck(ctx context.Context, input CreateCheckInput) (*types.WikaConflictCheck, error) {
	if !s.featureEnabled(ctx) {
		return nil, ErrFeatureDisabled
	}
	if s.store == nil {
		return nil, nil
	}
	trigger := strings.TrimSpace(input.Trigger)
	if trigger == "" {
		trigger = TriggerManual
	}
	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}
	check, err := s.store.CreateCheck(ctx, &types.WikaConflictCheck{
		TenantID:  input.TenantID,
		KBID:      strings.TrimSpace(input.KBID),
		Trigger:   trigger,
		Status:    CheckStatusPending,
		CreatedBy: strings.TrimSpace(input.ActorID),
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		return nil, err
	}
	if s.audit != nil && check != nil {
		if err := s.audit.Log(ctx, &types.AuditLog{
			TenantID:    input.TenantID,
			ActorUserID: strings.TrimSpace(input.ActorID),
			Action:      types.AuditActionWikaConflictCheckCreated,
			TargetType:  "wika_conflict_check",
			TargetID:    strconv.FormatUint(check.ID, 10),
		}); err != nil {
			return nil, err
		}
	}
	return check, nil
}

func (s *Service) ListItems(ctx context.Context, input ListItemsInput) ([]*types.WikaConflictItem, int64, error) {
	if !s.featureEnabled(ctx) {
		return nil, 0, ErrFeatureDisabled
	}
	if s.store == nil {
		return nil, 0, nil
	}
	return s.store.ListItems(ctx, input)
}

func (s *Service) ResolveItem(ctx context.Context, input ResolveItemInput) (*types.WikaConflictItem, error) {
	if !s.featureEnabled(ctx) {
		return nil, ErrFeatureDisabled
	}
	if s.store == nil {
		return nil, ErrConflictItemNotFound
	}
	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}
	input.Now = now
	item, err := s.store.ResolveItem(ctx, input)
	if err != nil {
		return nil, err
	}
	if s.audit != nil && item != nil {
		if err := s.audit.Log(ctx, &types.AuditLog{
			TenantID:    input.TenantID,
			ActorUserID: strings.TrimSpace(input.ActorID),
			Action:      conflictAuditActionForStatus(item.Status),
			TargetType:  "wika_conflict_item",
			TargetID:    strconv.FormatUint(item.ID, 10),
		}); err != nil {
			return nil, err
		}
	}
	return item, nil
}

func conflictAuditActionForStatus(status string) types.AuditAction {
	switch status {
	case ItemStatusConfirmed:
		return types.AuditActionWikaConflictItemConfirmed
	case ItemStatusDismissed:
		return types.AuditActionWikaConflictItemDismissed
	default:
		return types.AuditActionWikaConflictItemResolved
	}
}

type RunCheckInput struct {
	CheckID       uint64
	WorkerID      string
	Now           time.Time
	LeaseDuration time.Duration
}

func (s *Service) RunDueChecks(ctx context.Context, now time.Time) ([]*types.WikaConflictCheck, error) {
	if !s.featureEnabled(ctx) {
		return nil, ErrFeatureDisabled
	}
	if s.store == nil {
		return nil, nil
	}
	if now.IsZero() {
		now = time.Now()
	}
	return s.store.ListRunnableChecks(ctx, now, 20)
}

func (s *Service) RunCheck(ctx context.Context, input RunCheckInput) error {
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

func (s *Service) featureEnabled(ctx context.Context) bool {
	if s == nil || s.flags == nil {
		return false
	}
	return s.flags.GetBool(ctx, FeatureFlagKey, "", false)
}

func truncateError(msg string) string {
	msg = strings.TrimSpace(msg)
	if len([]rune(msg)) <= 512 {
		return msg
	}
	runes := []rune(msg)
	return string(runes[:512])
}
