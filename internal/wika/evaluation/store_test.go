package evaluation

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupEvaluationStoreTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&types.User{},
		&types.Tenant{},
		&types.KnowledgeBase{},
		&types.WikaEvalDataset{},
		&types.WikaEvalQAItem{},
		&types.WikaEvalRun{},
		&types.WikaEvalRunItem{},
	))
	return db
}

func TestGormEvaluationStoreCreatesDatasetAndQAItem(t *testing.T) {
	db := setupEvaluationStoreTestDB(t)
	require.NoError(t, db.Create(&types.User{ID: "owner", Username: "owner", Email: "owner@example.com", PasswordHash: "x"}).Error)
	require.NoError(t, db.Create(&types.Tenant{ID: 80, Name: "team", SpaceType: types.SpaceTypeTeam}).Error)
	require.NoError(t, db.Create(&types.KnowledgeBase{ID: "kb-team", TenantID: 80, CreatorID: "owner", Type: types.KnowledgeBaseTypeDocument}).Error)
	store := NewGormStore(db)

	dataset, err := store.CreateDataset(context.Background(), &types.WikaEvalDataset{
		TenantID:    80,
		KBID:        "kb-team",
		Name:        "团队检索黄金 QA",
		Description: "P2 smoke dataset",
		CreatedBy:   "owner",
	})
	require.NoError(t, err)
	if dataset.ID == 0 {
		t.Fatal("expected dataset id")
	}

	item, err := store.CreateQAItem(context.Background(), &types.WikaEvalQAItem{
		DatasetID:            dataset.ID,
		Question:             "如何排查索引延迟？",
		ExpectedAnswer:       "查看队列、embedding 任务和索引状态。",
		ExpectedKnowledgeIDs: types.JSON([]byte(`["k-1"]`)),
		ExpectedChunkIDs:     types.JSON([]byte(`["c-1"]`)),
		Tags:                 types.JSON([]byte(`["search"]`)),
		Enabled:              true,
		Version:              1,
	})
	require.NoError(t, err)
	if item.ID == 0 {
		t.Fatal("expected qa item id")
	}

	items, err := store.ListQAItems(context.Background(), dataset.ID, true)
	require.NoError(t, err)
	if len(items) != 1 || items[0].Question != "如何排查索引延迟？" {
		t.Fatalf("unexpected items: %+v", items)
	}
}

func TestGormEvaluationStoreCreatesRunItemsAndCompletesRun(t *testing.T) {
	db := setupEvaluationStoreTestDB(t)
	require.NoError(t, db.Create(&types.User{ID: "owner", Username: "owner", Email: "owner@example.com", PasswordHash: "x"}).Error)
	require.NoError(t, db.Create(&types.Tenant{ID: 80, Name: "team", SpaceType: types.SpaceTypeTeam}).Error)
	require.NoError(t, db.Create(&types.KnowledgeBase{ID: "kb-team", TenantID: 80, CreatorID: "owner", Type: types.KnowledgeBaseTypeDocument}).Error)
	store := NewGormStore(db)
	dataset, err := store.CreateDataset(context.Background(), &types.WikaEvalDataset{
		TenantID: 80, KBID: "kb-team", Name: "团队检索黄金 QA", CreatedBy: "owner",
	})
	require.NoError(t, err)
	qa, err := store.CreateQAItem(context.Background(), &types.WikaEvalQAItem{
		DatasetID: dataset.ID, Question: "Q1", ExpectedKnowledgeIDs: types.JSON([]byte(`["k-2"]`)), ExpectedChunkIDs: types.JSON([]byte("[]")), Tags: types.JSON([]byte("[]")), Enabled: true, Version: 1,
	})
	require.NoError(t, err)

	run, err := store.CreateRun(context.Background(), &types.WikaEvalRun{
		TenantID: 80, KBID: "kb-team", DatasetID: dataset.ID, DatasetVersion: 1, Trigger: "manual", Status: RunStatusRunning, CreatedBy: "owner",
	})
	require.NoError(t, err)
	require.NoError(t, store.SaveRunItems(context.Background(), []*types.WikaEvalRunItem{
		{RunID: run.ID, QAItemID: qa.ID, Hit: true, FirstHitRank: 2, RetrievedKnowledgeIDs: types.JSON([]byte(`["k-1","k-2"]`)), RetrievedChunkIDs: types.JSON([]byte("[]"))},
	}))

	completed, err := store.CompleteRun(context.Background(), run.ID, RunMetrics{Total: 1, RecallAt5: 1, MRR: 0.5, NDCGAt5: 0.630929}, 0)
	require.NoError(t, err)
	if completed.Status != RunStatusCompleted || completed.Total != 1 || completed.MRR != 0.5 {
		t.Fatalf("unexpected completed run: %+v", completed)
	}
	var count int64
	require.NoError(t, db.Model(&types.WikaEvalRunItem{}).Where("run_id = ?", run.ID).Count(&count).Error)
	if count != 1 {
		t.Fatalf("expected 1 run item, got %d", count)
	}
}
