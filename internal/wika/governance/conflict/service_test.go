package conflict

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

type fakeConflictStore struct {
	check          *types.WikaConflictCheck
	createdCheck   *types.WikaConflictCheck
	listItems      []*types.WikaConflictItem
	savedCheck     *types.WikaConflictCheck
	savedItems     []Candidate
	resolveInput   *ResolveItemInput
	resolvedItem   *types.WikaConflictItem
	completedCheck uint64
	failedCheck    uint64
}

func (s *fakeConflictStore) CreateCheck(ctx context.Context, check *types.WikaConflictCheck) (*types.WikaConflictCheck, error) {
	copied := *check
	copied.ID = 41
	s.createdCheck = &copied
	return &copied, nil
}

func (s *fakeConflictStore) ListItems(ctx context.Context, input ListItemsInput) ([]*types.WikaConflictItem, int64, error) {
	return s.listItems, int64(len(s.listItems)), nil
}

func (s *fakeConflictStore) ResolveItem(ctx context.Context, input ResolveItemInput) (*types.WikaConflictItem, error) {
	s.resolveInput = &input
	if s.resolvedItem != nil {
		item := *s.resolvedItem
		item.Status = input.Status
		item.ReviewerComment = input.Comment
		item.ResolvedBy = input.ActorID
		item.ResolvedAt = &input.Now
		return &item, nil
	}
	return nil, ErrConflictItemNotFound
}

func (s *fakeConflictStore) AcquireCheck(ctx context.Context, checkID uint64, workerID string, now time.Time, lease time.Duration) (*types.WikaConflictCheck, error) {
	if s.check == nil || s.check.ID != checkID {
		return nil, ErrCheckLeaseUnavailable
	}
	s.check.LockedBy = workerID
	s.check.Status = CheckStatusRunning
	return s.check, nil
}

func (s *fakeConflictStore) SaveConflictItems(ctx context.Context, check *types.WikaConflictCheck, items []Candidate) error {
	s.savedCheck = check
	s.savedItems = items
	return nil
}

func (s *fakeConflictStore) CompleteCheck(ctx context.Context, checkID uint64, workerID string, now time.Time) error {
	s.completedCheck = checkID
	return nil
}

func (s *fakeConflictStore) FailCheck(ctx context.Context, checkID uint64, workerID string, now time.Time, errMsg string) error {
	s.failedCheck = checkID
	return nil
}

type fakeConflictGenerator struct {
	input      GenerateInput
	candidates []Candidate
	err        error
}

func (g *fakeConflictGenerator) GenerateCandidates(ctx context.Context, input GenerateInput) ([]Candidate, error) {
	g.input = input
	return g.candidates, g.err
}

type fakeConflictAudit struct {
	entries []*types.AuditLog
}

func (a *fakeConflictAudit) Log(ctx context.Context, entry *types.AuditLog) error {
	a.entries = append(a.entries, entry)
	return nil
}

func TestConflictServiceCreateCheckWritesAudit(t *testing.T) {
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	store := &fakeConflictStore{}
	audit := &fakeConflictAudit{}
	svc := &Service{store: store, audit: audit}

	got, err := svc.CreateCheck(context.Background(), CreateCheckInput{
		ActorID:  "u-owner",
		TenantID: 80,
		KBID:     "kb-team",
		Trigger:  TriggerManual,
		Now:      now,
	})
	if err != nil {
		t.Fatalf("CreateCheck returned error: %v", err)
	}
	if got.ID != 41 || store.createdCheck == nil || store.createdCheck.Status != CheckStatusPending || store.createdCheck.CreatedBy != "u-owner" {
		t.Fatalf("unexpected created check: got=%+v stored=%+v", got, store.createdCheck)
	}
	if len(audit.entries) != 1 ||
		audit.entries[0].Action != types.AuditActionWikaConflictCheckCreated ||
		audit.entries[0].TenantID != 80 ||
		audit.entries[0].ActorUserID != "u-owner" ||
		audit.entries[0].TargetID != "41" {
		t.Fatalf("unexpected audit entries: %+v", audit.entries)
	}
}

func TestConflictServiceResolveItemWritesAudit(t *testing.T) {
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	store := &fakeConflictStore{resolvedItem: &types.WikaConflictItem{ID: 7, TenantID: 80, KBID: "kb-team", Status: ItemStatusOpen}}
	audit := &fakeConflictAudit{}
	svc := &Service{store: store, audit: audit}

	got, err := svc.ResolveItem(context.Background(), ResolveItemInput{
		ActorID:  "u-reviewer",
		TenantID: 80,
		ItemID:   7,
		Status:   ItemStatusResolved,
		Comment:  "已合并到团队手册",
		Now:      now,
	})
	if err != nil {
		t.Fatalf("ResolveItem returned error: %v", err)
	}
	if got.Status != ItemStatusResolved || got.ResolvedBy != "u-reviewer" || store.resolveInput == nil || store.resolveInput.Comment != "已合并到团队手册" {
		t.Fatalf("unexpected resolved item: got=%+v input=%+v", got, store.resolveInput)
	}
	if len(audit.entries) != 1 ||
		audit.entries[0].Action != types.AuditActionWikaConflictItemResolved ||
		audit.entries[0].TenantID != 80 ||
		audit.entries[0].ActorUserID != "u-reviewer" ||
		audit.entries[0].TargetID != "7" {
		t.Fatalf("unexpected audit entries: %+v", audit.entries)
	}
}

func TestConflictServiceRunCheckGeneratesCandidatesAndCompletes(t *testing.T) {
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	store := &fakeConflictStore{check: &types.WikaConflictCheck{ID: 41, TenantID: 80, KBID: "kb-team", Trigger: TriggerManual, Status: CheckStatusPending, CreatedBy: "u-owner"}}
	generator := &fakeConflictGenerator{candidates: []Candidate{
		{
			SourceKnowledgeID: "k-1",
			TargetKnowledgeID: "k-2",
			ConflictType:      ConflictTypeContradiction,
			ConfidenceScore:   0.91,
			Evidence:          types.JSON([]byte(`{"source_hash":"a","target_hash":"b"}`)),
			AIExplanation:     "两条知识结论相反",
		},
	}}
	svc := &Service{store: store, generator: generator}

	err := svc.RunCheck(context.Background(), RunCheckInput{
		CheckID:       41,
		WorkerID:      "worker-1",
		Now:           now,
		LeaseDuration: time.Minute,
	})
	if err != nil {
		t.Fatalf("RunCheck returned error: %v", err)
	}
	if generator.input.TenantID != 80 || generator.input.KBID != "kb-team" || generator.input.Trigger != TriggerManual {
		t.Fatalf("unexpected generator input: %+v", generator.input)
	}
	if store.savedCheck == nil || store.savedCheck.ID != 41 || len(store.savedItems) != 1 {
		t.Fatalf("expected generated candidate to be saved, check=%+v items=%+v", store.savedCheck, store.savedItems)
	}
	if store.completedCheck != 41 || store.failedCheck != 0 {
		t.Fatalf("expected check completed, completed=%d failed=%d", store.completedCheck, store.failedCheck)
	}
}
