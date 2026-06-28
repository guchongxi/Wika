package freshness

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

type fakeFreshnessStore struct {
	states map[string]*types.WikaKnowledgeState
	access map[string]*types.KnowledgeAccessDaily
	check  *types.WikaFreshnessCheck
	items  []*types.WikaFreshnessCheckItem
}

func (s *fakeFreshnessStore) ListKnowledgeStates(ctx context.Context, tenantID uint64, kbID string) ([]*types.WikaKnowledgeState, error) {
	out := make([]*types.WikaKnowledgeState, 0, len(s.states))
	for _, state := range s.states {
		out = append(out, state)
	}
	return out, nil
}

func (s *fakeFreshnessStore) LastAccess(ctx context.Context, knowledgeID string) (*types.KnowledgeAccessDaily, error) {
	return s.access[knowledgeID], nil
}

func (s *fakeFreshnessStore) SaveCheck(ctx context.Context, check *types.WikaFreshnessCheck, items []*types.WikaFreshnessCheckItem) (*types.WikaFreshnessCheck, error) {
	copied := *check
	copied.ID = 41
	s.check = &copied
	s.items = items
	return &copied, nil
}

func TestRunCheckCreatesFreshnessItemsForAllMVPConditions(t *testing.T) {
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	expired := now.Add(-24 * time.Hour)
	expiring := now.Add(7 * 24 * time.Hour)
	lowConfidence := 0.4
	store := &fakeFreshnessStore{
		states: map[string]*types.WikaKnowledgeState{
			"k-expired":        {KnowledgeID: "k-expired", TenantID: 80, KBID: "kb-team", ExpiresAt: &expired, QualityScore: 90},
			"k-expiring":       {KnowledgeID: "k-expiring", TenantID: 80, KBID: "kb-team", ExpiresAt: &expiring, QualityScore: 90},
			"k-low-quality":    {KnowledgeID: "k-low-quality", TenantID: 80, KBID: "kb-team", QualityScore: 30},
			"k-low-confidence": {KnowledgeID: "k-low-confidence", TenantID: 80, KBID: "kb-team", QualityScore: 90, ConfidenceScore: &lowConfidence},
			"k-stale":          {KnowledgeID: "k-stale", TenantID: 80, KBID: "kb-team", QualityScore: 90},
		},
		access: map[string]*types.KnowledgeAccessDaily{
			"k-stale": {KnowledgeID: "k-stale", LastAccessedAt: now.Add(-100 * 24 * time.Hour)},
		},
	}
	svc := &Service{store: store}

	got, err := svc.RunCheck(context.Background(), RunCheckInput{
		TenantID: 80,
		KBID:     "kb-team",
		Trigger:  "manual",
		Now:      now,
	})
	if err != nil {
		t.Fatalf("RunCheck returned error: %v", err)
	}
	if got.ID != 41 || store.check == nil || store.check.Status != CheckStatusCompleted {
		t.Fatalf("unexpected check: %+v", got)
	}
	seen := map[string]bool{}
	for _, item := range store.items {
		seen[item.IssueType] = true
	}
	for _, issueType := range []string{IssueExpired, IssueExpiring, IssueLowQuality, IssueLowConfidence, IssueStale} {
		if !seen[issueType] {
			t.Fatalf("expected issue %s in items: %+v", issueType, store.items)
		}
	}
}
