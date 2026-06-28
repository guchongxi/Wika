package freshness

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

type fakeFreshnessStore struct {
	states       map[string]*types.WikaKnowledgeState
	access       map[string]*types.KnowledgeAccessDaily
	check        *types.WikaFreshnessCheck
	items        []*types.WikaFreshnessCheckItem
	handleUpdate *ItemUpdate
	handledItem  *types.WikaFreshnessCheckItem
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

func (s *fakeFreshnessStore) ListChecks(ctx context.Context, tenantID uint64, kbID string) ([]*types.WikaFreshnessCheck, error) {
	return []*types.WikaFreshnessCheck{s.check}, nil
}

func (s *fakeFreshnessStore) ListItems(ctx context.Context, tenantID uint64, kbID, status string) ([]*types.WikaFreshnessCheckItem, error) {
	return s.items, nil
}

func (s *fakeFreshnessStore) HandleItem(ctx context.Context, update ItemUpdate) (*types.WikaFreshnessCheckItem, error) {
	s.handleUpdate = &update
	item := *s.handledItem
	item.PreviousStatus = ItemStatusOpen
	item.Status = update.Status
	item.ResolutionAction = update.Action
	item.ResolutionNote = update.Note
	item.ResolvedBy = update.ActorID
	item.ResolvedAt = &update.Now
	return &item, nil
}

type fakeFreshnessAudit struct {
	entries []*types.AuditLog
}

func (a *fakeFreshnessAudit) Log(ctx context.Context, entry *types.AuditLog) error {
	a.entries = append(a.entries, entry)
	return nil
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

func TestHandleItemMarksUpdatedAndWritesAudit(t *testing.T) {
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	store := &fakeFreshnessStore{
		handledItem: &types.WikaFreshnessCheckItem{
			ID:          7,
			TenantID:    80,
			KBID:        "kb-team",
			KnowledgeID: "k-expired",
			IssueType:   IssueExpired,
			Status:      ItemStatusOpen,
		},
	}
	audit := &fakeFreshnessAudit{}
	svc := &Service{store: store, audit: audit}

	got, err := svc.HandleItem(context.Background(), HandleItemInput{
		ActorID:  "u-reviewer",
		TenantID: 80,
		ItemID:   7,
		Action:   ActionMarkUpdated,
		Note:     "已补充最新证据",
		Now:      now,
	})
	if err != nil {
		t.Fatalf("HandleItem returned error: %v", err)
	}
	if got.Status != ItemStatusResolved || got.PreviousStatus != ItemStatusOpen || got.ResolutionAction != ActionMarkUpdated {
		t.Fatalf("unexpected handled item: %+v", got)
	}
	if store.handleUpdate == nil ||
		store.handleUpdate.KnowledgeFreshnessStatus != FreshnessStatusFresh ||
		store.handleUpdate.ActorID != "u-reviewer" ||
		store.handleUpdate.Note != "已补充最新证据" {
		t.Fatalf("unexpected handle update: %+v", store.handleUpdate)
	}
	if len(audit.entries) != 1 {
		t.Fatalf("expected one audit entry, got %d", len(audit.entries))
	}
	entry := audit.entries[0]
	if entry.Action != types.AuditActionWikaFreshnessItemHandled ||
		entry.TenantID != 80 ||
		entry.ActorUserID != "u-reviewer" ||
		entry.TargetType != "freshness" ||
		entry.TargetID != "7" {
		t.Fatalf("unexpected audit entry: %+v", entry)
	}
}
