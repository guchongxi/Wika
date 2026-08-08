package service

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupModelScopeServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&types.Model{}))
	return db
}

func TestSetSystemDefaultModelMakesModelSelectableAndUniquePerType(t *testing.T) {
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))
	db := setupModelScopeServiceTestDB(t)
	repo := repository.NewModelRepository(db)
	require.NoError(t, db.Create(&types.Model{
		ID: "old-default", TenantID: types.DefaultBuiltinModelTenantID, Name: "old",
		Type: types.ModelTypeKnowledgeQA, Source: types.ModelSourceRemote,
		Scope: types.ModelScopeSystem, IsBuiltin: true, IsDefault: true, UserSelectable: true,
		Status: types.ModelStatusActive,
	}).Error)
	require.NoError(t, db.Create(&types.Model{
		ID: "new-default", TenantID: types.DefaultBuiltinModelTenantID, Name: "new",
		Type: types.ModelTypeKnowledgeQA, Source: types.ModelSourceRemote,
		Scope: types.ModelScopeSystem, IsBuiltin: true, UserSelectable: false,
		Status: types.ModelStatusActive,
	}).Error)

	svc := NewModelService(repo, nil, nil, nil, nil, nil)
	updated, err := svc.SetSystemDefaultModel(ctx, "new-default")
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.True(t, updated.IsDefault)
	assert.True(t, updated.UserSelectable)

	old, err := repo.GetSystemByID(ctx, "old-default")
	require.NoError(t, err)
	require.NotNil(t, old)
	assert.False(t, old.IsDefault)
}

func TestSetSystemModelSelectableRejectsDisablingDefault(t *testing.T) {
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))
	db := setupModelScopeServiceTestDB(t)
	repo := repository.NewModelRepository(db)
	require.NoError(t, db.Create(&types.Model{
		ID: "default-model", TenantID: types.DefaultBuiltinModelTenantID, Name: "default",
		Type: types.ModelTypeEmbedding, Source: types.ModelSourceRemote,
		Scope: types.ModelScopeSystem, IsBuiltin: true, IsDefault: true, UserSelectable: true,
		Status: types.ModelStatusActive,
	}).Error)

	svc := NewModelService(repo, nil, nil, nil, nil, nil)
	_, err := svc.SetSystemModelSelectable(ctx, "default-model", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "default")
}

func TestCreateUserModelForcesPrivateOwnership(t *testing.T) {
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))
	db := setupModelScopeServiceTestDB(t)
	repo := repository.NewModelRepository(db)
	svc := NewModelService(repo, nil, nil, nil, nil, nil)

	model := &types.Model{
		Name: "mine", Type: types.ModelTypeKnowledgeQA, Source: types.ModelSourceRemote,
		Scope: types.ModelScopeSystem, IsBuiltin: true, OwnerUserID: "attacker",
		UserSelectable: true, IsDefault: true,
	}
	require.NoError(t, svc.CreateUserModel(ctx, "u-a", model))

	got, err := repo.GetByIDForUser(ctx, 7, "u-a", model.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, types.ModelScopeUser, got.Scope)
	assert.Equal(t, "u-a", got.OwnerUserID)
	assert.False(t, got.IsBuiltin)
	assert.False(t, got.UserSelectable)
	assert.False(t, got.IsDefault)
}
