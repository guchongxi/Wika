package space

import (
	"context"
	"errors"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupSpaceStoreTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&types.User{},
		&types.Tenant{},
		&types.TenantMember{},
		&types.UserPersonalSpace{},
		&types.KnowledgeBase{},
		&types.WikaSpaceDefault{},
		&types.WikaSpacePolicy{},
	))
	return db
}

func TestGormStoreGetPersonalSpaceNotFound(t *testing.T) {
	db := setupSpaceStoreTestDB(t)
	store := NewGormStore(db)

	tenant, err := store.GetPersonalSpace(context.Background(), "user-1")

	if tenant != nil {
		t.Fatalf("expected nil tenant, got %+v", tenant)
	}
	if !errors.Is(err, ErrPersonalSpaceNotFound) {
		t.Fatalf("expected ErrPersonalSpaceNotFound, got %v", err)
	}
}

func TestGormStoreCreatePersonalSpacePersistsTenantOwnerAndMapping(t *testing.T) {
	db := setupSpaceStoreTestDB(t)
	require.NoError(t, db.Create(&types.User{
		ID:           "user-1",
		Username:     "user1",
		Email:        "user1@example.com",
		PasswordHash: "x",
	}).Error)
	store := NewGormStore(db)

	tenant, err := store.CreatePersonalSpace(
		context.Background(),
		"user-1",
		&types.Tenant{Name: "顾测的个人空间", SpaceType: types.SpaceTypePersonal, Status: "active"},
		&types.TenantMember{UserID: "user-1", Role: types.TenantRoleOwner, Status: types.TenantMemberStatusActive},
	)

	require.NoError(t, err)
	if tenant.ID == 0 {
		t.Fatal("expected created tenant id")
	}

	var mapping types.UserPersonalSpace
	require.NoError(t, db.First(&mapping, "user_id = ?", "user-1").Error)
	if mapping.TenantID != tenant.ID {
		t.Fatalf("expected mapping tenant %d, got %d", tenant.ID, mapping.TenantID)
	}

	var member types.TenantMember
	require.NoError(t, db.First(&member, "user_id = ? AND tenant_id = ?", "user-1", tenant.ID).Error)
	if member.Role != types.TenantRoleOwner {
		t.Fatalf("expected owner role, got %q", member.Role)
	}

	found, err := store.GetPersonalSpace(context.Background(), "user-1")
	require.NoError(t, err)
	if found.ID != tenant.ID || found.SpaceType != types.SpaceTypePersonal {
		t.Fatalf("expected persisted personal tenant, got %+v", found)
	}
}

func TestGormStoreCreatePersonalSpaceCreatesDefaultKBAndPolicy(t *testing.T) {
	db := setupSpaceStoreTestDB(t)
	require.NoError(t, db.Create(&types.User{
		ID:           "user-1",
		Username:     "user1",
		Email:        "user1@example.com",
		PasswordHash: "x",
	}).Error)
	store := NewGormStore(db)

	tenant, err := store.CreatePersonalSpace(
		context.Background(),
		"user-1",
		&types.Tenant{Name: "顾测的个人空间", SpaceType: types.SpaceTypePersonal, Status: "active"},
		&types.TenantMember{UserID: "user-1", Role: types.TenantRoleOwner, Status: types.TenantMemberStatusActive},
	)

	require.NoError(t, err)

	var defaults types.WikaSpaceDefault
	require.NoError(t, db.First(&defaults, "tenant_id = ?", tenant.ID).Error)
	if defaults.CreatedBy != "user-1" {
		t.Fatalf("expected default KB creator user-1, got %q", defaults.CreatedBy)
	}

	var kb types.KnowledgeBase
	require.NoError(t, db.First(&kb, "id = ?", defaults.DefaultKBID).Error)
	if kb.TenantID != tenant.ID || kb.CreatorID != "user-1" || kb.Type != types.KnowledgeBaseTypeDocument {
		t.Fatalf("unexpected default KB: %+v", kb)
	}

	var policy types.WikaSpacePolicy
	require.NoError(t, db.First(&policy, "tenant_id = ?", tenant.ID).Error)
	if policy.AutoApplyApproved {
		t.Fatal("expected auto_apply_approved to default false")
	}
	if policy.PolicyVersion != 1 {
		t.Fatalf("expected policy version 1, got %d", policy.PolicyVersion)
	}
}
