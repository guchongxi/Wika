package conflict

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestWorkerRunOnceProcessesPendingCheckAndCreatesDuplicateItem(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:wika_conflict_worker?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&types.Knowledge{},
		&types.WikaConflictCheck{},
		&types.WikaConflictItem{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	insertManualKnowledge := func(id, content string) {
		t.Helper()
		knowledge := &types.Knowledge{
			ID:              id,
			TenantID:        80,
			KnowledgeBaseID: "kb-team",
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
	insertManualKnowledge("k-worker-1", "重复知识：服务启动失败时检查全局默认模型。")
	insertManualKnowledge("k-worker-2", "重复知识：服务启动失败时检查全局默认模型。")

	store := NewGormStore(db)
	flags := &fakeConflictFeatureGate{enabled: true}
	svc := &Service{
		store:     store,
		generator: NewDeterministicCandidateGenerator(db),
		flags:     flags,
	}
	check, err := svc.CreateCheck(context.Background(), CreateCheckInput{
		ActorID:  "u-owner",
		TenantID: 80,
		KBID:     "kb-team",
		Trigger:  TriggerManual,
		Now:      time.Date(2026, 6, 30, 2, 45, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateCheck returned error: %v", err)
	}

	worker := NewWorker(svc, flags, WithWorkerID("worker-test"), WithWorkerLeaseDuration(time.Minute))
	if err := worker.RunOnce(context.Background(), time.Date(2026, 6, 30, 2, 46, 0, 0, time.UTC)); err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}

	var loaded types.WikaConflictCheck
	if err := db.First(&loaded, "id = ?", check.ID).Error; err != nil {
		t.Fatalf("load check: %v", err)
	}
	if loaded.Status != CheckStatusCompleted {
		t.Fatalf("expected completed check, got %+v", loaded)
	}
	items, total, err := store.ListItems(context.Background(), ListItemsInput{
		TenantID: 80,
		KBID:     "kb-team",
		Status:   ItemStatusOpen,
	})
	if err != nil {
		t.Fatalf("ListItems returned error: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("expected one open duplicate item, total=%d items=%+v", total, items)
	}
	if items[0].ConflictType != ConflictTypeDuplicate {
		t.Fatalf("expected duplicate item, got %+v", items[0])
	}
}
