package search

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupSearchStoreTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&types.UserPersonalSpace{},
		&types.WikaSpaceDefault{},
		&types.Tenant{},
		&types.TenantMember{},
		&types.KnowledgeBase{},
		&types.Knowledge{},
		&types.Organization{},
		&types.OrganizationTenantMember{},
		&types.WikaOrgShare{},
		&types.KnowledgeAccessDaily{},
	))
	require.NoError(t, db.Create(&types.Tenant{ID: 80, Name: "team", SpaceType: types.SpaceTypeTeam}).Error)
	require.NoError(t, db.Create(&types.KnowledgeBase{ID: "kb-team", TenantID: 80, Type: types.KnowledgeBaseTypeDocument}).Error)
	require.NoError(t, db.Create(&types.Knowledge{ID: "k-1", TenantID: 80, KnowledgeBaseID: "kb-team", Title: "知识"}).Error)
	return db
}

func TestGormSearchStoreListReadableScopesIncludesActiveOrgShares(t *testing.T) {
	db := setupSearchStoreTestDB(t)
	require.NoError(t, db.Create(&types.Tenant{ID: 90, Name: "target", SpaceType: types.SpaceTypeTeam}).Error)
	require.NoError(t, db.Create(&types.TenantMember{UserID: "u-target", TenantID: 90, Role: types.TenantRoleViewer, Status: types.TenantMemberStatusActive}).Error)
	require.NoError(t, db.Create(&types.Organization{ID: "org-1", Name: "org"}).Error)
	require.NoError(t, db.Create(&types.OrganizationTenantMember{ID: "otm-source", OrganizationID: "org-1", TenantID: 80, Role: types.OrgRoleAdmin}).Error)
	require.NoError(t, db.Create(&types.OrganizationTenantMember{ID: "otm-target", OrganizationID: "org-1", TenantID: 90, Role: types.OrgRoleViewer}).Error)
	allowedFields, err := json.Marshal([]string{"id", "title"})
	require.NoError(t, err)
	require.NoError(t, db.Create(&types.WikaOrgShare{
		OrgID:          "org-1",
		SourceTenantID: 80,
		SourceKBID:     "kb-team",
		TargetTenantID: 90,
		Mode:           types.WikaOrgShareModeReference,
		AllowedFields:  types.JSON(allowedFields),
		Status:         types.WikaOrgShareStatusActive,
		CreatedBy:      "u-source",
	}).Error)
	require.NoError(t, db.Create(&types.WikaOrgShare{
		OrgID:          "org-1",
		SourceTenantID: 80,
		SourceKBID:     "kb-team",
		TargetTenantID: 90,
		Mode:           types.WikaOrgShareModeReference,
		AllowedFields:  types.JSON(allowedFields),
		Status:         types.WikaOrgShareStatusRevoked,
		CreatedBy:      "u-source",
	}).Error)

	scopes, err := NewGormStore(db).ListReadableScopes(context.Background(), "u-target", true)
	require.NoError(t, err)

	var shared []ReadableScope
	for _, scope := range scopes {
		if scope.Source == SourceShared {
			shared = append(shared, scope)
		}
	}
	if len(shared) != 1 {
		t.Fatalf("expected one active shared scope, got %+v", scopes)
	}
	if shared[0].TenantID != 80 || shared[0].KBID != "kb-team" {
		t.Fatalf("unexpected shared scope: %+v", shared[0])
	}
	require.Equal(t, []string{"id", "title"}, shared[0].AllowedFields)

	scopes, err = NewGormStore(db).ListReadableScopes(context.Background(), "u-target", false)
	require.NoError(t, err)
	for _, scope := range scopes {
		if scope.Source == SourceShared {
			t.Fatalf("shared scope must follow includeTeam=false, got %+v", scopes)
		}
	}
}

func TestGormSearchStoreIgnoresOrgShareWhenTargetNotOrgMember(t *testing.T) {
	db := setupSearchStoreTestDB(t)
	require.NoError(t, db.Create(&types.Tenant{ID: 90, Name: "target", SpaceType: types.SpaceTypeTeam}).Error)
	require.NoError(t, db.Create(&types.TenantMember{UserID: "u-target", TenantID: 90, Role: types.TenantRoleViewer, Status: types.TenantMemberStatusActive}).Error)
	require.NoError(t, db.Create(&types.Organization{ID: "org-1", Name: "org"}).Error)
	require.NoError(t, db.Create(&types.OrganizationTenantMember{ID: "otm-source", OrganizationID: "org-1", TenantID: 80, Role: types.OrgRoleAdmin}).Error)
	allowedFields, err := json.Marshal([]string{"id", "title"})
	require.NoError(t, err)
	require.NoError(t, db.Create(&types.WikaOrgShare{
		OrgID:          "org-1",
		SourceTenantID: 80,
		SourceKBID:     "kb-team",
		TargetTenantID: 90,
		Mode:           types.WikaOrgShareModeReference,
		AllowedFields:  types.JSON(allowedFields),
		Status:         types.WikaOrgShareStatusActive,
		CreatedBy:      "u-source",
	}).Error)

	scopes, err := NewGormStore(db).ListReadableScopes(context.Background(), "u-target", true)
	require.NoError(t, err)
	for _, scope := range scopes {
		if scope.Source == SourceShared {
			t.Fatalf("shared scope must require target tenant to belong to org, got %+v", scopes)
		}
	}
}

func TestGormSearchStoreSanitizesUnsafeSharedAllowedFields(t *testing.T) {
	db := setupSearchStoreTestDB(t)
	require.NoError(t, db.Create(&types.Tenant{ID: 90, Name: "target", SpaceType: types.SpaceTypeTeam}).Error)
	require.NoError(t, db.Create(&types.TenantMember{UserID: "u-target", TenantID: 90, Role: types.TenantRoleViewer, Status: types.TenantMemberStatusActive}).Error)
	require.NoError(t, db.Create(&types.Organization{ID: "org-1", Name: "org"}).Error)
	require.NoError(t, db.Create(&types.OrganizationTenantMember{ID: "otm-source", OrganizationID: "org-1", TenantID: 80, Role: types.OrgRoleAdmin}).Error)
	require.NoError(t, db.Create(&types.OrganizationTenantMember{ID: "otm-target", OrganizationID: "org-1", TenantID: 90, Role: types.OrgRoleViewer}).Error)
	allowedFields, err := json.Marshal([]string{"id", "title", "content", "file", "updated_at", "freshness_status"})
	require.NoError(t, err)
	require.NoError(t, db.Create(&types.WikaOrgShare{
		OrgID:          "org-1",
		SourceTenantID: 80,
		SourceKBID:     "kb-team",
		TargetTenantID: 90,
		Mode:           types.WikaOrgShareModeReference,
		AllowedFields:  types.JSON(allowedFields),
		Status:         types.WikaOrgShareStatusActive,
		CreatedBy:      "u-source",
	}).Error)

	scopes, err := NewGormStore(db).ListReadableScopes(context.Background(), "u-target", true)
	require.NoError(t, err)

	var shared *ReadableScope
	for i := range scopes {
		if scopes[i].Source == SourceShared {
			shared = &scopes[i]
			break
		}
	}
	if shared == nil {
		t.Fatalf("expected shared scope, got %+v", scopes)
	}
	require.Equal(t, []string{"id", "title", "updated_at", "freshness_status"}, shared.AllowedFields)
}

func TestGormSearchStoreRecordAccessUpsertsDailyRollup(t *testing.T) {
	db := setupSearchStoreTestDB(t)
	store := NewGormStore(db)
	first := time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC)
	second := time.Date(2026, 6, 29, 18, 0, 0, 0, time.UTC)

	require.NoError(t, store.RecordAccess(context.Background(), []AccessRecord{
		{TenantID: 80, KBID: "kb-team", KnowledgeID: "k-1", AccessedAt: first},
	}))
	require.NoError(t, store.RecordAccess(context.Background(), []AccessRecord{
		{TenantID: 80, KBID: "kb-team", KnowledgeID: "k-1", AccessedAt: second},
	}))

	var access types.KnowledgeAccessDaily
	require.NoError(t, db.First(&access, "tenant_id = ? AND kb_id = ? AND knowledge_id = ?", 80, "kb-team", "k-1").Error)
	if access.AccessCount != 2 {
		t.Fatalf("expected access_count=2, got %d", access.AccessCount)
	}
	if !access.LastAccessedAt.Equal(second) {
		t.Fatalf("expected last_accessed_at=%s, got %s", second, access.LastAccessedAt)
	}
}

func TestGormSearchStoreRecordAccessDoesNotTouchKnowledgeMainTable(t *testing.T) {
	db := setupSearchStoreTestDB(t)
	originalUpdatedAt := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	require.NoError(t, db.Model(&types.Knowledge{}).
		Where("id = ?", "k-1").
		Update("updated_at", originalUpdatedAt).Error)

	store := NewGormStore(db)
	require.NoError(t, store.RecordAccess(context.Background(), []AccessRecord{
		{TenantID: 80, KBID: "kb-team", KnowledgeID: "k-1", AccessedAt: time.Date(2026, 6, 29, 18, 0, 0, 0, time.UTC)},
	}))

	var knowledge types.Knowledge
	require.NoError(t, db.First(&knowledge, "id = ?", "k-1").Error)
	if !knowledge.UpdatedAt.Equal(originalUpdatedAt) {
		t.Fatalf("RecordAccess must not update knowledges.updated_at, got %s", knowledge.UpdatedAt)
	}
}
