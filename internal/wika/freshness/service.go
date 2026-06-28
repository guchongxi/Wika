package freshness

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

var ErrFreshnessStoreNotConfigured = errors.New("freshness store not configured")

// Store 隔离保鲜扫描需要的数据访问。
type Store interface {
	ListKnowledgeStates(ctx context.Context, tenantID uint64, kbID string) ([]*types.WikaKnowledgeState, error)
	LastAccess(ctx context.Context, knowledgeID string) (*types.KnowledgeAccessDaily, error)
	SaveCheck(ctx context.Context, check *types.WikaFreshnessCheck, items []*types.WikaFreshnessCheckItem) (*types.WikaFreshnessCheck, error)
}

// Service 负责从知识状态和访问聚合生成保鲜问题。
type Service struct {
	store Store
}

func NewService(store *GormStore) *Service {
	return &Service{store: store}
}

// RunCheck 扫描团队 KB 中过期、将过期、低质、低置信和长期未访问知识。
func (s *Service) RunCheck(ctx context.Context, input RunCheckInput) (*types.WikaFreshnessCheck, error) {
	if s.store == nil {
		return nil, ErrFreshnessStoreNotConfigured
	}
	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}
	trigger := strings.TrimSpace(input.Trigger)
	if trigger == "" {
		trigger = CheckTriggerManual
	}
	states, err := s.store.ListKnowledgeStates(ctx, input.TenantID, strings.TrimSpace(input.KBID))
	if err != nil {
		return nil, err
	}
	items := make([]*types.WikaFreshnessCheckItem, 0)
	for _, state := range states {
		items = append(items, s.itemsForState(ctx, input.TenantID, strings.TrimSpace(input.KBID), state, now)...)
	}
	completedAt := now
	return s.store.SaveCheck(ctx, &types.WikaFreshnessCheck{
		TenantID:    input.TenantID,
		KBID:        strings.TrimSpace(input.KBID),
		Trigger:     trigger,
		Status:      CheckStatusCompleted,
		CheckedAt:   now,
		CompletedAt: &completedAt,
		CreatedAt:   now,
	}, items)
}

func (s *Service) itemsForState(ctx context.Context, tenantID uint64, kbID string, state *types.WikaKnowledgeState, now time.Time) []*types.WikaFreshnessCheckItem {
	if state == nil || state.KnowledgeID == "" {
		return nil
	}
	items := make([]*types.WikaFreshnessCheckItem, 0, 2)
	if state.ExpiresAt != nil {
		switch {
		case !state.ExpiresAt.After(now):
			items = append(items, newItem(tenantID, kbID, state.KnowledgeID, IssueExpired, SeverityHigh, "renew_or_deprecate"))
		case state.ExpiresAt.Sub(now) <= expiringWindow:
			items = append(items, newItem(tenantID, kbID, state.KnowledgeID, IssueExpiring, SeverityMedium, "extend_expiry"))
		}
	}
	if state.QualityScore > 0 && state.QualityScore < lowQualityScore {
		items = append(items, newItem(tenantID, kbID, state.KnowledgeID, IssueLowQuality, SeverityHigh, "review_quality"))
	}
	if state.ConfidenceScore != nil && *state.ConfidenceScore < lowConfidenceRate {
		items = append(items, newItem(tenantID, kbID, state.KnowledgeID, IssueLowConfidence, SeverityMedium, "verify_evidence"))
	}
	if access, err := s.store.LastAccess(ctx, state.KnowledgeID); err == nil && access != nil && !access.LastAccessedAt.IsZero() {
		if now.Sub(access.LastAccessedAt) >= staleWindow {
			items = append(items, newItem(tenantID, kbID, state.KnowledgeID, IssueStale, SeverityMedium, "review_usage"))
		}
	}
	return items
}

func newItem(tenantID uint64, kbID, knowledgeID, issueType, severity, action string) *types.WikaFreshnessCheckItem {
	return &types.WikaFreshnessCheckItem{
		TenantID:        tenantID,
		KBID:            kbID,
		KnowledgeID:     knowledgeID,
		IssueType:       issueType,
		Severity:        severity,
		SuggestedAction: action,
		Status:          ItemStatusOpen,
	}
}
