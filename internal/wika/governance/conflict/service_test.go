package conflict

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

type fakeConflictStore struct {
	check          *types.WikaConflictCheck
	savedCheck     *types.WikaConflictCheck
	savedItems     []Candidate
	completedCheck uint64
	failedCheck    uint64
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
