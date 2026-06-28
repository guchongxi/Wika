package evaluation

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

type fakeEvaluationStore struct {
	dataset *types.WikaEvalDataset
	items   []*types.WikaEvalQAItem
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
