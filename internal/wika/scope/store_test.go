package scope

import (
	"context"
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
	require.NoError(t, db.AutoMigrate(&types.Tenant{}, &types.TenantMember{}, &types.KnowledgeBase{}))
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
