package freshness

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupFreshnessStoreTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&types.Tenant{},
		&types.KnowledgeBase{},
		&types.Knowledge{},
		&types.WikaKnowledgeState{},
		&types.KnowledgeAccessDaily{},
		&types.WikaFreshnessCheck{},
		&types.WikaFreshnessCheckItem{},
	))
	return db
}

func TestGormFreshnessStoreSavesCheckItems(t *testing.T) {
	db := setupFreshnessStoreTestDB(t)
	require.NoError(t, db.Create(&types.Tenant{ID: 80, Name: "team", SpaceType: types.SpaceTypeTeam}).Error)
	require.NoError(t, db.Create(&types.KnowledgeBase{ID: "kb-team", TenantID: 80, Type: types.KnowledgeBaseTypeDocument}).Error)
	require.NoError(t, db.Create(&types.Knowledge{ID: "k-1", TenantID: 80, KnowledgeBaseID: "kb-team", Title: "知识"}).Error)
	require.NoError(t, db.Create(&types.WikaKnowledgeState{KnowledgeID: "k-1", TenantID: 80, KBID: "kb-team", QualityScore: 30, FreshnessStatus: "fresh", ReviewStatus: "none"}).Error)
	require.NoError(t, db.Create(&types.KnowledgeAccessDaily{TenantID: 80, KBID: "kb-team", KnowledgeID: "k-1", Day: time.Now(), AccessCount: 1, LastAccessedAt: time.Now()}).Error)
	store := NewGormStore(db)

	states, err := store.ListKnowledgeStates(context.Background(), 80, "kb-team")
	require.NoError(t, err)
	if len(states) != 1 || states[0].KnowledgeID != "k-1" {
		t.Fatalf("unexpected states: %+v", states)
	}
	access, err := store.LastAccess(context.Background(), "k-1")
	require.NoError(t, err)
	if access == nil || access.KnowledgeID != "k-1" {
		t.Fatalf("unexpected access: %+v", access)
	}

	check, err := store.SaveCheck(context.Background(), &types.WikaFreshnessCheck{
		TenantID: 80, KBID: "kb-team", Trigger: "manual", Status: CheckStatusCompleted,
	}, []*types.WikaFreshnessCheckItem{
		{TenantID: 80, KBID: "kb-team", KnowledgeID: "k-1", IssueType: IssueLowQuality, Severity: SeverityHigh, SuggestedAction: "review", Status: ItemStatusOpen},
	})
	require.NoError(t, err)
	if check.ID == 0 {
		t.Fatal("expected check id")
	}
	var count int64
	require.NoError(t, db.Model(&types.WikaFreshnessCheckItem{}).Where("check_id = ?", check.ID).Count(&count).Error)
	if count != 1 {
		t.Fatalf("expected 1 item, got %d", count)
	}
	var previousStatus *string
	require.NoError(t, db.Raw("SELECT previous_status FROM freshness_check_items WHERE check_id = ?", check.ID).Scan(&previousStatus).Error)
	if previousStatus != nil {
		t.Fatalf("expected initial previous_status to be NULL, got %q", *previousStatus)
	}
}

func TestGormFreshnessStoreHandlesItemAndUpdatesKnowledgeState(t *testing.T) {
	db := setupFreshnessStoreTestDB(t)
	require.NoError(t, db.Create(&types.Tenant{ID: 80, Name: "team", SpaceType: types.SpaceTypeTeam}).Error)
	require.NoError(t, db.Create(&types.KnowledgeBase{ID: "kb-team", TenantID: 80, Type: types.KnowledgeBaseTypeDocument}).Error)
	require.NoError(t, db.Create(&types.Knowledge{ID: "k-1", TenantID: 80, KnowledgeBaseID: "kb-team", Title: "知识"}).Error)
	require.NoError(t, db.Create(&types.WikaKnowledgeState{KnowledgeID: "k-1", TenantID: 80, KBID: "kb-team", QualityScore: 30, FreshnessStatus: "expired", ReviewStatus: "needs_review"}).Error)
	check := &types.WikaFreshnessCheck{TenantID: 80, KBID: "kb-team", Trigger: CheckTriggerManual, Status: CheckStatusCompleted, CheckedAt: time.Now()}
	require.NoError(t, db.Create(check).Error)
	item := &types.WikaFreshnessCheckItem{CheckID: check.ID, TenantID: 80, KBID: "kb-team", KnowledgeID: "k-1", IssueType: IssueExpired, Severity: SeverityHigh, SuggestedAction: "renew_or_deprecate", Status: ItemStatusOpen}
	require.NoError(t, db.Create(item).Error)
	store := NewGormStore(db)

	handled, err := store.HandleItem(context.Background(), ItemUpdate{
		TenantID:                 80,
		ItemID:                   item.ID,
		Action:                   ActionMarkUpdated,
		Note:                     "已更新",
		ActorID:                  "u-reviewer",
		Status:                   ItemStatusResolved,
		KnowledgeFreshnessStatus: FreshnessStatusFresh,
		Now:                      time.Now(),
	})
	require.NoError(t, err)
	if handled.Status != ItemStatusResolved ||
		handled.PreviousStatus != ItemStatusOpen ||
		handled.ResolutionAction != ActionMarkUpdated ||
		handled.ResolvedBy != "u-reviewer" ||
		handled.ResolvedAt == nil {
		t.Fatalf("unexpected handled item: %+v", handled)
	}
	var state types.WikaKnowledgeState
	require.NoError(t, db.First(&state, "knowledge_id = ?", "k-1").Error)
	if state.FreshnessStatus != FreshnessStatusFresh {
		t.Fatalf("expected freshness status fresh, got %s", state.FreshnessStatus)
	}
}
