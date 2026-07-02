package conflict

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestDeterministicCandidateGeneratorDetectsDuplicateManualKnowledgeInSameKB(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:wika_conflict_generator?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&types.Knowledge{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	insertManualKnowledge := func(id, kbID, content string, tenantID uint64) {
		t.Helper()
		knowledge := &types.Knowledge{
			ID:              id,
			TenantID:        tenantID,
			KnowledgeBaseID: kbID,
			Type:            types.KnowledgeTypeManual,
			Title:           id,
			ParseStatus:     types.ParseStatusCompleted,
			EnableStatus:    types.ManualKnowledgeStatusPublish,
		}
		if err := knowledge.SetManualMetadata(types.NewManualKnowledgeMetadata(content, types.ManualKnowledgeStatusPublish, 1)); err != nil {
			t.Fatalf("set metadata: %v", err)
		}
		if err := db.Create(knowledge).Error; err != nil {
			t.Fatalf("create knowledge %s: %v", id, err)
		}
	}

	insertManualKnowledge("k-1", "kb-team", "服务启动失败时，先检查全局默认模型和 KB defaults。", 80)
	insertManualKnowledge("k-2", "kb-team", " 服务启动失败时，先检查全局默认模型和 KB defaults。\n", 80)
	insertManualKnowledge("k-3", "kb-team", "这是另一条完全不同的知识。", 80)
	insertManualKnowledge("k-other-kb", "kb-other", "服务启动失败时，先检查全局默认模型和 KB defaults。", 80)
	insertManualKnowledge("k-other-tenant", "kb-team", "服务启动失败时，先检查全局默认模型和 KB defaults。", 81)

	generator := NewDeterministicCandidateGenerator(db)
	got, err := generator.GenerateCandidates(context.Background(), GenerateInput{
		CheckID:  7,
		TenantID: 80,
		KBID:     "kb-team",
		Trigger:  TriggerManual,
	})
	if err != nil {
		t.Fatalf("GenerateCandidates returned error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected one duplicate candidate, got %+v", got)
	}
	candidate := got[0]
	if candidate.SourceKnowledgeID != "k-1" || candidate.TargetKnowledgeID != "k-2" {
		t.Fatalf("expected duplicate pair k-1/k-2, got %+v", candidate)
	}
	if candidate.ConflictType != ConflictTypeDuplicate {
		t.Fatalf("expected duplicate conflict type, got %q", candidate.ConflictType)
	}
	if candidate.ConfidenceScore < 0.99 {
		t.Fatalf("expected high confidence duplicate, got %v", candidate.ConfidenceScore)
	}
	var evidence map[string]any
	if err := json.Unmarshal(candidate.Evidence, &evidence); err != nil {
		t.Fatalf("evidence is not JSON: %v", err)
	}
	if evidence["detector"] != "deterministic_content_hash" || evidence["normalized_hash"] == "" {
		t.Fatalf("unexpected evidence: %+v", evidence)
	}
	if candidate.AIExplanation == "" {
		t.Fatalf("expected explanation")
	}
}

func TestDeterministicCandidateGeneratorUsesManualPublishMetadataEvenWhenRowIsPending(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:wika_conflict_generator_pending?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&types.Knowledge{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	for _, id := range []string{"k-pending-1", "k-pending-2"} {
		knowledge := &types.Knowledge{
			ID:              id,
			TenantID:        80,
			KnowledgeBaseID: "kb-team",
			Type:            types.KnowledgeTypeManual,
			Title:           id,
			ParseStatus:     types.ParseStatusPending,
			EnableStatus:    "disabled",
		}
		if err := knowledge.SetManualMetadata(types.NewManualKnowledgeMetadata("发布中的手工知识也应进入冲突检测。", types.ManualKnowledgeStatusPublish, 1)); err != nil {
			t.Fatalf("set metadata: %v", err)
		}
		if err := db.Create(knowledge).Error; err != nil {
			t.Fatalf("create knowledge %s: %v", id, err)
		}
	}

	got, err := NewDeterministicCandidateGenerator(db).GenerateCandidates(context.Background(), GenerateInput{
		TenantID: 80,
		KBID:     "kb-team",
	})
	if err != nil {
		t.Fatalf("GenerateCandidates returned error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected pending published manual knowledge to be checked, got %+v", got)
	}
}
