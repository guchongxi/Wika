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
	runs      []*types.WikaEvalRun
	runItems  []*types.WikaEvalRunItem
	completed *types.WikaEvalRun
	getTenant uint64
	getKB     string
	getID     uint64
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

func (s *fakeEvaluationStore) GetDataset(ctx context.Context, tenantID uint64, kbID string, datasetID uint64) (*types.WikaEvalDataset, error) {
	s.getTenant = tenantID
	s.getKB = kbID
	s.getID = datasetID
	return s.dataset, nil
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

func (s *fakeEvaluationStore) ListRuns(ctx context.Context, tenantID uint64, kbID string, limit int) ([]*types.WikaEvalRun, error) {
	return s.runs, nil
}

type fakeSearchRunner struct {
	results map[string][]string
	calls   []wikasearch.SearchInput
}

func (r *fakeSearchRunner) SearchKnowledge(ctx context.Context, input wikasearch.SearchInput) (*wikasearch.SearchResult, error) {
	r.calls = append(r.calls, input)
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

func TestAddQAItemChecksDatasetScopeWhenProvided(t *testing.T) {
	store := &fakeEvaluationStore{
		dataset: &types.WikaEvalDataset{ID: 11, TenantID: 80, KBID: "kb-team", Name: "黄金 QA"},
	}
	svc := &Service{store: store}

	if _, err := svc.AddQAItem(context.Background(), AddQAItemInput{
		TenantID:             80,
		KBID:                 "kb-team",
		DatasetID:            11,
		Question:             "如何排查索引延迟？",
		ExpectedKnowledgeIDs: []string{"k-1"},
	}); err != nil {
		t.Fatalf("AddQAItem returned error: %v", err)
	}
	if store.getTenant != 80 || store.getKB != "kb-team" || store.getID != 11 {
		t.Fatalf("dataset scope was not checked: tenant=%d kb=%s id=%d", store.getTenant, store.getKB, store.getID)
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
		dataset: &types.WikaEvalDataset{ID: 11, TenantID: 80, KBID: "kb-team", Name: "黄金 QA"},
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
	if store.getTenant != 80 || store.getKB != "kb-team" || store.getID != 11 {
		t.Fatalf("run did not check dataset scope: tenant=%d kb=%s id=%d", store.getTenant, store.getKB, store.getID)
	}
}

func TestDryRunSearchesQuestionWithoutPersistingRun(t *testing.T) {
	runner := &fakeSearchRunner{results: map[string][]string{
		"如何排查默认模型？": []string{"k-1", "k-2"},
	}}
	svc := &Service{search: runner}

	got, err := svc.DryRun(context.Background(), DryRunInput{
		ActorID:  "owner",
		TenantID: 80,
		KBID:     "kb-team",
		Question: "如何排查默认模型？",
		Limit:    3,
	})
	if err != nil {
		t.Fatalf("DryRun returned error: %v", err)
	}
	if got.Question != "如何排查默认模型？" || len(got.Results) != 2 || got.Results[0].KnowledgeID != "k-1" {
		t.Fatalf("unexpected dry-run result: %+v", got)
	}
	if len(runner.calls) != 1 ||
		runner.calls[0].UserID != "owner" ||
		runner.calls[0].TenantID != 80 ||
		runner.calls[0].Limit != 3 ||
		!runner.calls[0].IncludeTeam {
		t.Fatalf("unexpected search call: %+v", runner.calls)
	}
}

func TestExportDatasetOmitsExpectedAnswers(t *testing.T) {
	store := &fakeEvaluationStore{
		dataset: &types.WikaEvalDataset{ID: 11, TenantID: 80, KBID: "kb-team", Name: "黄金 QA"},
		items: []*types.WikaEvalQAItem{
			{
				ID:                   21,
				DatasetID:            11,
				Question:             "如何排查索引延迟？",
				ExpectedAnswer:       "敏感答案不应导出",
				ExpectedKnowledgeIDs: types.JSON([]byte(`["k-1"]`)),
				ExpectedChunkIDs:     types.JSON([]byte(`["c-1"]`)),
				Tags:                 types.JSON([]byte(`["search"]`)),
				Enabled:              true,
				Version:              2,
			},
		},
	}
	svc := &Service{store: store}

	got, err := svc.ExportDataset(context.Background(), ExportDatasetInput{
		TenantID:  80,
		KBID:      "kb-team",
		DatasetID: 11,
	})
	if err != nil {
		t.Fatalf("ExportDataset returned error: %v", err)
	}
	if got.Dataset.ID != 11 || len(got.Items) != 1 {
		t.Fatalf("unexpected export result: %+v", got)
	}
	if got.Items[0].Question != "如何排查索引延迟？" || len(got.Items[0].ExpectedKnowledgeIDs) != 1 {
		t.Fatalf("expected question and ids in export, got %+v", got.Items[0])
	}
	if got.Items[0].ExpectedAnswer != "" {
		t.Fatalf("expected answer must be omitted from export, got %q", got.Items[0].ExpectedAnswer)
	}
}

func TestImportDatasetCreatesItemsAfterScopeCheck(t *testing.T) {
	store := &fakeEvaluationStore{
		dataset: &types.WikaEvalDataset{ID: 11, TenantID: 80, KBID: "kb-team", Name: "黄金 QA"},
	}
	svc := &Service{store: store}

	got, err := svc.ImportDataset(context.Background(), ImportDatasetInput{
		TenantID:  80,
		KBID:      "kb-team",
		DatasetID: 11,
		Items: []ImportQAItem{
			{
				Question:             "如何排查默认模型？",
				ExpectedAnswer:       "查看系统默认 KB 配置。",
				ExpectedKnowledgeIDs: []string{"k-1"},
				ExpectedChunkIDs:     []string{"c-1"},
				Tags:                 []string{"p2"},
				Enabled:              true,
			},
			{
				Question: "只用于 dry-run 的问题",
				Enabled:  true,
			},
		},
	})
	if err != nil {
		t.Fatalf("ImportDataset returned error: %v", err)
	}
	if got.Imported != 2 || len(got.Items) != 2 {
		t.Fatalf("unexpected import result: %+v", got)
	}
	if len(store.items) != 2 || store.items[0].DatasetID != 11 || store.items[0].Question != "如何排查默认模型？" {
		t.Fatalf("items were not imported: %+v", store.items)
	}
}

func TestTrendReturnsRecentRunsForKB(t *testing.T) {
	store := &fakeEvaluationStore{
		runs: []*types.WikaEvalRun{
			{ID: 32, TenantID: 80, KBID: "kb-team", DatasetID: 11, Status: RunStatusCompleted, RecallAt5: 0.8, MRR: 0.5, NDCGAt5: 0.7, Total: 5, Failed: 1, Trigger: RunTriggerManual},
			{ID: 31, TenantID: 80, KBID: "kb-team", DatasetID: 11, Status: RunStatusCompleted, RecallAt5: 0.6, MRR: 0.4, NDCGAt5: 0.5, Total: 5, Failed: 0, Trigger: RunTriggerSchedule},
		},
	}
	svc := &Service{store: store}

	got, err := svc.Trend(context.Background(), TrendInput{
		TenantID: 80,
		KBID:     "kb-team",
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("Trend returned error: %v", err)
	}
	if len(got.Runs) != 2 || got.Runs[0].RunID != 32 || got.Runs[0].RecallAt5 != 0.8 {
		t.Fatalf("unexpected trend result: %+v", got)
	}
}
