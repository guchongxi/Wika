package evaluation

import (
	"context"
	"math"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	wikasearch "github.com/Tencent/WeKnora/internal/wika/search"
)

type fakeEvaluationStore struct {
	dataset   *types.WikaEvalDataset
	items     []*types.WikaEvalQAItem
	run       *types.WikaEvalRun
	runItems  []*types.WikaEvalRunItem
	completed *types.WikaEvalRun
}

func (s *fakeEvaluationStore) CreateDataset(ctx context.Context, item *types.WikaEvalDataset) (*types.WikaEvalDataset, error) {
	copied := *item
	copied.ID = 11
	s.dataset = &copied
	return &copied, nil
}

func (s *fakeEvaluationStore) CreateQAItem(ctx context.Context, item *types.WikaEvalQAItem) (*types.WikaEvalQAItem, error) {
	copied := *item
	copied.ID = 21
	s.items = append(s.items, &copied)
	return &copied, nil
}

func (s *fakeEvaluationStore) ListQAItems(ctx context.Context, datasetID uint64, enabledOnly bool) ([]*types.WikaEvalQAItem, error) {
	return s.items, nil
}

func (s *fakeEvaluationStore) CreateRun(ctx context.Context, item *types.WikaEvalRun) (*types.WikaEvalRun, error) {
	copied := *item
	copied.ID = 31
	s.run = &copied
	return &copied, nil
}

func (s *fakeEvaluationStore) SaveRunItems(ctx context.Context, items []*types.WikaEvalRunItem) error {
	s.runItems = items
	return nil
}

func (s *fakeEvaluationStore) CompleteRun(ctx context.Context, runID uint64, metrics RunMetrics, failed int) (*types.WikaEvalRun, error) {
	copied := *s.run
	copied.Status = RunStatusCompleted
	copied.MRR = metrics.MRR
	copied.RecallAt5 = metrics.RecallAt5
	copied.NDCGAt5 = metrics.NDCGAt5
	copied.Total = metrics.Total
	copied.Failed = failed
	s.completed = &copied
	return &copied, nil
}

type fakeSearchRunner struct {
	results map[string][]string
}

func (r *fakeSearchRunner) SearchKnowledge(ctx context.Context, input wikasearch.SearchInput) (*wikasearch.SearchResult, error) {
	ids := r.results[input.Query]
	items := make([]wikasearch.ResultItem, 0, len(ids))
	for _, id := range ids {
		items = append(items, wikasearch.ResultItem{KnowledgeID: id, Title: id})
	}
	return &wikasearch.SearchResult{Results: items}, nil
}

func TestCreateDatasetStoresTenantKBAndActor(t *testing.T) {
	store := &fakeEvaluationStore{}
	svc := &Service{store: store}

	got, err := svc.CreateDataset(context.Background(), CreateDatasetInput{
		ActorID:     "owner",
		TenantID:    80,
		KBID:        "kb-team",
		Name:        "团队检索黄金 QA",
		Description: "P2 smoke dataset",
	})
	if err != nil {
		t.Fatalf("CreateDataset returned error: %v", err)
	}
	if got.ID != 11 || got.TenantID != 80 || got.KBID != "kb-team" || got.CreatedBy != "owner" {
		t.Fatalf("unexpected dataset: %+v", got)
	}
}

func TestAddQAItemPersistsExpectedIDs(t *testing.T) {
	store := &fakeEvaluationStore{}
	svc := &Service{store: store}

	got, err := svc.AddQAItem(context.Background(), AddQAItemInput{
		DatasetID:            11,
		Question:             "如何排查索引延迟？",
		ExpectedAnswer:       "查看队列、embedding 任务和索引状态。",
		ExpectedKnowledgeIDs: []string{"k-1"},
		ExpectedChunkIDs:     []string{"c-1"},
		Tags:                 []string{"search"},
	})
	if err != nil {
		t.Fatalf("AddQAItem returned error: %v", err)
	}
	if got.ID != 21 || got.DatasetID != 11 || got.Question == "" {
		t.Fatalf("unexpected qa item: %+v", got)
	}
	if !jsonArrayContains(got.ExpectedKnowledgeIDs, "k-1") || !jsonArrayContains(got.ExpectedChunkIDs, "c-1") {
		t.Fatalf("expected ids not persisted: %+v", got)
	}
}

func TestValidateDatasetForFormalRunRejectsQAWithoutExpectedIDs(t *testing.T) {
	store := &fakeEvaluationStore{
		items: []*types.WikaEvalQAItem{
			{ID: 1, DatasetID: 11, Question: "只有问题", Enabled: true, ExpectedKnowledgeIDs: types.JSON([]byte("[]")), ExpectedChunkIDs: types.JSON([]byte("[]"))},
		},
	}
	svc := &Service{store: store}

	if err := svc.ValidateDatasetForFormalRun(context.Background(), 11); err == nil {
		t.Fatal("expected missing expected ids to reject formal run")
	}
}

func TestRunEvaluationPersistsRunItemsAndMetrics(t *testing.T) {
	store := &fakeEvaluationStore{
		items: []*types.WikaEvalQAItem{
			{ID: 1, DatasetID: 11, Question: "Q1", Enabled: true, ExpectedKnowledgeIDs: types.JSON([]byte(`["k-2"]`)), ExpectedChunkIDs: types.JSON([]byte("[]"))},
			{ID: 2, DatasetID: 11, Question: "Q2", Enabled: true, ExpectedKnowledgeIDs: types.JSON([]byte(`["k-9"]`)), ExpectedChunkIDs: types.JSON([]byte("[]"))},
		},
	}
	runner := &fakeSearchRunner{results: map[string][]string{
		"Q1": []string{"k-1", "k-2"},
		"Q2": []string{"k-1", "k-3"},
	}}
	svc := &Service{store: store, search: runner}

	got, err := svc.RunEvaluation(context.Background(), RunInput{
		ActorID:   "owner",
		TenantID:  80,
		KBID:      "kb-team",
		DatasetID: 11,
	})
	if err != nil {
		t.Fatalf("RunEvaluation returned error: %v", err)
	}
	if got.ID != 31 || got.Status != RunStatusCompleted || got.Total != 2 || got.Failed != 0 {
		t.Fatalf("unexpected run: %+v", got)
	}
	if len(store.runItems) != 2 {
		t.Fatalf("expected 2 run items, got %+v", store.runItems)
	}
	if !store.runItems[0].Hit || store.runItems[0].FirstHitRank != 2 || store.runItems[1].Hit {
		t.Fatalf("unexpected run items: %+v", store.runItems)
	}
	if math.Abs(got.RecallAt5-0.5) > 0.000001 || math.Abs(got.MRR-0.25) > 0.000001 || got.NDCGAt5 <= 0 {
		t.Fatalf("unexpected metrics: recall=%f mrr=%f ndcg=%f", got.RecallAt5, got.MRR, got.NDCGAt5)
	}
}
