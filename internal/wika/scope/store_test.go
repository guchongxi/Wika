package scope

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupScopeStoreTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&types.Tenant{}, &types.TenantMember{}, &types.KnowledgeBase{}, &types.Organization{}, &types.OrganizationTenantMember{}, &types.WikaOrgShare{}))
	return db
}

func TestGormStoreResolvesKnowledgeBaseTenantAndMember(t *testing.T) {
	db := setupScopeStoreTestDB(t)
	require.NoError(t, db.Create(&types.Tenant{ID: 7, Name: "team", SpaceType: types.SpaceTypeTeam}).Error)
	require.NoError(t, db.Create(&types.KnowledgeBase{ID: "kb-1", Name: "kb", TenantID: 7}).Error)
	require.NoError(t, db.Create(&types.TenantMember{
		UserID:   "user-1",
		TenantID: 7,
		Role:     types.TenantRoleViewer,
		Status:   types.TenantMemberStatusActive,
	}).Error)
	store := NewGormStore(db)

	kb, err := store.GetKnowledgeBase(context.Background(), "kb-1")
	require.NoError(t, err)
	if kb.TenantID != 7 {
		t.Fatalf("expected tenant 7, got %d", kb.TenantID)
	}

	tenant, err := store.GetTenant(context.Background(), 7)
	require.NoError(t, err)
	if tenant.SpaceType != types.SpaceTypeTeam {
		t.Fatalf("expected team tenant, got %q", tenant.SpaceType)
	}

	member, err := store.GetTenantMember(context.Background(), "user-1", 7)
	require.NoError(t, err)
	if member.Role != types.TenantRoleViewer {
		t.Fatalf("expected viewer role, got %q", member.Role)
	}
}

func TestGormStoreMapsMissingRowsToResourceNotFound(t *testing.T) {
	db := setupScopeStoreTestDB(t)
	store := NewGormStore(db)

	_, err := store.GetKnowledgeBase(context.Background(), "missing")
	if !errors.Is(err, ErrResourceNotFound) {
		t.Fatalf("expected ErrResourceNotFound for missing KB, got %v", err)
	}
	_, err = store.GetTenant(context.Background(), 7)
	if !errors.Is(err, ErrResourceNotFound) {
		t.Fatalf("expected ErrResourceNotFound for missing tenant, got %v", err)
	}
	_, err = store.GetTenantMember(context.Background(), "user-1", 7)
	if !errors.Is(err, ErrResourceNotFound) {
		t.Fatalf("expected ErrResourceNotFound for missing member, got %v", err)
	}
}

func TestGormStoreListsActiveSharedKnowledgeBaseScopes(t *testing.T) {
	db := setupScopeStoreTestDB(t)
	require.NoError(t, db.Create(&types.Tenant{ID: 80, Name: "source", SpaceType: types.SpaceTypeTeam}).Error)
	require.NoError(t, db.Create(&types.Tenant{ID: 90, Name: "target", SpaceType: types.SpaceTypeTeam}).Error)
	require.NoError(t, db.Create(&types.KnowledgeBase{ID: "kb-shared", Name: "shared", TenantID: 80, Type: types.KnowledgeBaseTypeDocument}).Error)
	require.NoError(t, db.Create(&types.TenantMember{UserID: "user-target", TenantID: 90, Role: types.TenantRoleViewer, Status: types.TenantMemberStatusActive}).Error)
	require.NoError(t, db.Create(&types.Organization{ID: "org-1", Name: "org"}).Error)
	require.NoError(t, db.Create(&types.OrganizationTenantMember{ID: "otm-source", OrganizationID: "org-1", TenantID: 80, Role: types.OrgRoleAdmin}).Error)
	require.NoError(t, db.Create(&types.OrganizationTenantMember{ID: "otm-target", OrganizationID: "org-1", TenantID: 90, Role: types.OrgRoleViewer}).Error)
	allowedFields, err := json.Marshal([]string{"id", "title"})
	require.NoError(t, err)
	require.NoError(t, db.Create(&types.WikaOrgShare{
		OrgID:          "org-1",
		SourceTenantID: 80,
		SourceKBID:     "kb-shared",
		TargetTenantID: 90,
		Mode:           types.WikaOrgShareModeReference,
		AllowedFields:  types.JSON(allowedFields),
		Status:         types.WikaOrgShareStatusActive,
		CreatedBy:      "user-source",
	}).Error)
	require.NoError(t, db.Create(&types.WikaOrgShare{
		OrgID:          "org-1",
		SourceTenantID: 80,
		SourceKBID:     "kb-shared",
		TargetTenantID: 90,
		Mode:           types.WikaOrgShareModeReference,
		AllowedFields:  types.JSON(allowedFields),
		Status:         types.WikaOrgShareStatusRevoked,
		CreatedBy:      "user-source",
	}).Error)

	scopes, err := NewGormStore(db).ListSharedKnowledgeBaseScopes(context.Background(), "user-target", "kb-shared")

	require.NoError(t, err)
	if len(scopes) != 1 {
		t.Fatalf("expected one active shared scope, got %+v", scopes)
	}
	if scopes[0].Source != ScopeSourceShared || scopes[0].TenantID != 80 || scopes[0].KBID != "kb-shared" {
		t.Fatalf("unexpected shared scope: %+v", scopes[0])
	}
	require.Equal(t, []string{"id", "title"}, scopes[0].AllowedFields)
}

func TestGormStoreIgnoresSharedScopeWhenTargetNotOrgMember(t *testing.T) {
	db := setupScopeStoreTestDB(t)
	require.NoError(t, db.Create(&types.Tenant{ID: 80, Name: "source", SpaceType: types.SpaceTypeTeam}).Error)
	require.NoError(t, db.Create(&types.Tenant{ID: 90, Name: "target", SpaceType: types.SpaceTypeTeam}).Error)
	require.NoError(t, db.Create(&types.KnowledgeBase{ID: "kb-shared", Name: "shared", TenantID: 80, Type: types.KnowledgeBaseTypeDocument}).Error)
	require.NoError(t, db.Create(&types.TenantMember{UserID: "user-target", TenantID: 90, Role: types.TenantRoleViewer, Status: types.TenantMemberStatusActive}).Error)
	require.NoError(t, db.Create(&types.Organization{ID: "org-1", Name: "org"}).Error)
	require.NoError(t, db.Create(&types.OrganizationTenantMember{ID: "otm-source", OrganizationID: "org-1", TenantID: 80, Role: types.OrgRoleAdmin}).Error)
	allowedFields, err := json.Marshal([]string{"id", "title"})
	require.NoError(t, err)
	require.NoError(t, db.Create(&types.WikaOrgShare{
		OrgID:          "org-1",
		SourceTenantID: 80,
		SourceKBID:     "kb-shared",
		TargetTenantID: 90,
		Mode:           types.WikaOrgShareModeReference,
		AllowedFields:  types.JSON(allowedFields),
		Status:         types.WikaOrgShareStatusActive,
		CreatedBy:      "user-source",
	}).Error)

	scopes, err := NewGormStore(db).ListSharedKnowledgeBaseScopes(context.Background(), "user-target", "kb-shared")

	require.NoError(t, err)
	if len(scopes) != 0 {
		t.Fatalf("shared scope must require target tenant to belong to org, got %+v", scopes)
	}
}

func TestGormStoreSanitizesUnsafeSharedAllowedFields(t *testing.T) {
	db := setupScopeStoreTestDB(t)
	require.NoError(t, db.Create(&types.Tenant{ID: 80, Name: "source", SpaceType: types.SpaceTypeTeam}).Error)
	require.NoError(t, db.Create(&types.Tenant{ID: 90, Name: "target", SpaceType: types.SpaceTypeTeam}).Error)
	require.NoError(t, db.Create(&types.KnowledgeBase{ID: "kb-shared", Name: "shared", TenantID: 80, Type: types.KnowledgeBaseTypeDocument}).Error)
	require.NoError(t, db.Create(&types.TenantMember{UserID: "user-target", TenantID: 90, Role: types.TenantRoleViewer, Status: types.TenantMemberStatusActive}).Error)
	require.NoError(t, db.Create(&types.Organization{ID: "org-1", Name: "org"}).Error)
	require.NoError(t, db.Create(&types.OrganizationTenantMember{ID: "otm-source", OrganizationID: "org-1", TenantID: 80, Role: types.OrgRoleAdmin}).Error)
	require.NoError(t, db.Create(&types.OrganizationTenantMember{ID: "otm-target", OrganizationID: "org-1", TenantID: 90, Role: types.OrgRoleViewer}).Error)
	allowedFields, err := json.Marshal([]string{"id", "title", "file", "content", "updated_at", "quality_score"})
	require.NoError(t, err)
	require.NoError(t, db.Create(&types.WikaOrgShare{
		OrgID:          "org-1",
		SourceTenantID: 80,
		SourceKBID:     "kb-shared",
		TargetTenantID: 90,
		Mode:           types.WikaOrgShareModeReference,
		AllowedFields:  types.JSON(allowedFields),
		Status:         types.WikaOrgShareStatusActive,
		CreatedBy:      "user-source",
	}).Error)

	scopes, err := NewGormStore(db).ListSharedKnowledgeBaseScopes(context.Background(), "user-target", "kb-shared")

	require.NoError(t, err)
	require.Len(t, scopes, 1)
	require.Equal(t, []string{"id", "title", "updated_at", "quality_score"}, scopes[0].AllowedFields)
}
