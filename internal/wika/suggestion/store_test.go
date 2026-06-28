package suggestion

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupSuggestionStoreTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&types.User{},
		&types.Tenant{},
		&types.TenantMember{},
		&types.KnowledgeBase{},
		&types.Knowledge{},
		&types.WikaSpaceDefault{},
		&types.WikaSpacePolicy{},
		&types.WikaKnowledgeSuggestion{},
		&types.WikaKnowledgeLineage{},
	))
	return db
}

func TestGormSuggestionStoreFindsDefaultsPolicyAndMembers(t *testing.T) {
	db := setupSuggestionStoreTestDB(t)
	require.NoError(t, db.Create(&types.User{ID: "user-1", Username: "user1", Email: "u1@example.com", PasswordHash: "x"}).Error)
	require.NoError(t, db.Create(&types.Tenant{ID: 70, Name: "personal", SpaceType: types.SpaceTypePersonal}).Error)
	require.NoError(t, db.Create(&types.Tenant{ID: 80, Name: "team", SpaceType: types.SpaceTypeTeam}).Error)
	require.NoError(t, db.Create(&types.TenantMember{UserID: "user-1", TenantID: 80, Role: types.TenantRoleContributor, Status: types.TenantMemberStatusActive}).Error)
	require.NoError(t, db.Create(&types.KnowledgeBase{ID: "kb-team", TenantID: 80, CreatorID: "user-1", Type: types.KnowledgeBaseTypeDocument}).Error)
	require.NoError(t, db.Create(&types.WikaSpaceDefault{TenantID: 80, DefaultKBID: "kb-team", CreatedBy: "user-1"}).Error)
	require.NoError(t, db.Create(&types.WikaSpacePolicy{TenantID: 80, AutoApplyApproved: true, PolicyVersion: 3}).Error)
	store := NewGormStore(db)

	member, err := store.GetTenantMember(context.Background(), "user-1", 80)
	require.NoError(t, err)
	if member == nil || member.Role != types.TenantRoleContributor {
		t.Fatalf("expected active contributor member, got %+v", member)
	}

	defaultKB, err := store.GetDefaultKB(context.Background(), 80)
	require.NoError(t, err)
	if defaultKB != "kb-team" {
		t.Fatalf("expected default KB kb-team, got %q", defaultKB)
	}

	policy, err := store.GetSpacePolicy(context.Background(), 80)
	require.NoError(t, err)
	if !policy.AutoApplyApproved || policy.PolicyVersion != 3 {
		t.Fatalf("unexpected policy: %+v", policy)
	}
}

func TestGormSuggestionStoreSavesAndFindsByIdempotencyKey(t *testing.T) {
	db := setupSuggestionStoreTestDB(t)
	require.NoError(t, db.Create(&types.User{ID: "user-1", Username: "user1", Email: "u1@example.com", PasswordHash: "x"}).Error)
	require.NoError(t, db.Create(&types.Tenant{ID: 70, Name: "personal", SpaceType: types.SpaceTypePersonal}).Error)
	require.NoError(t, db.Create(&types.Tenant{ID: 80, Name: "team", SpaceType: types.SpaceTypeTeam}).Error)
	require.NoError(t, db.Create(&types.KnowledgeBase{ID: "kb-personal", TenantID: 70, CreatorID: "user-1", Type: types.KnowledgeBaseTypeDocument}).Error)
	require.NoError(t, db.Create(&types.KnowledgeBase{ID: "kb-team", TenantID: 80, CreatorID: "user-1", Type: types.KnowledgeBaseTypeDocument}).Error)
	require.NoError(t, db.Create(&types.Knowledge{ID: "knowledge-1", TenantID: 70, KnowledgeBaseID: "kb-personal", Title: "排查记录"}).Error)
	store := NewGormStore(db)

	saved, err := store.SaveSuggestion(context.Background(), &types.WikaKnowledgeSuggestion{
		SourceTenantID:    70,
		SourceKBID:        "kb-personal",
		SourceKnowledgeID: "knowledge-1",
		TargetTenantID:    80,
		TargetKBID:        "kb-team",
		SubmitterID:       "user-1",
		IdempotencyKey:    "idem-1",
		AIDecision:        string(DecisionApproved),
		AIConfidence:      0.9,
		AIReview:          types.JSON([]byte(`{"decision":"approved","confidence":0.9,"risks":[]}`)),
		FinalDecision:     string(DecisionApproved),
		Status:            string(StatusAIReviewed),
		PolicyVersion:     1,
	})
	require.NoError(t, err)
	if saved.ID == 0 {
		t.Fatal("expected saved suggestion id")
	}

	existing, err := store.FindByIdempotencyKey(context.Background(), "user-1", 80, "idem-1")
	require.NoError(t, err)
	if existing == nil || existing.SuggestionID != saved.ID || existing.AIDecision != DecisionApproved || existing.Status != StatusAIReviewed {
		t.Fatalf("unexpected idempotency result: %+v", existing)
	}
}
