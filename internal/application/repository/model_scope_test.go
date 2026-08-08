package repository

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupModelScopeTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&types.Model{}))
	return db
}

func modelScopeIDs(models []*types.Model) []string {
	ids := make([]string, 0, len(models))
	for _, model := range models {
		ids = append(ids, model.ID)
	}
	return ids
}

func seedModelScopeRows(t *testing.T, db *gorm.DB) {
	t.Helper()
	rows := []*types.Model{
		{ID: "system-visible", TenantID: types.DefaultBuiltinModelTenantID, Name: "visible", Type: types.ModelTypeKnowledgeQA, Source: types.ModelSourceRemote, Scope: types.ModelScopeSystem, IsBuiltin: true, UserSelectable: true, Status: types.ModelStatusActive},
		{ID: "system-hidden", TenantID: types.DefaultBuiltinModelTenantID, Name: "hidden", Type: types.ModelTypeKnowledgeQA, Source: types.ModelSourceRemote, Scope: types.ModelScopeSystem, IsBuiltin: true, UserSelectable: false, Status: types.ModelStatusActive},
		{ID: "system-default", TenantID: types.DefaultBuiltinModelTenantID, Name: "default", Type: types.ModelTypeKnowledgeQA, Source: types.ModelSourceRemote, Scope: types.ModelScopeSystem, IsBuiltin: true, IsDefault: true, Status: types.ModelStatusActive},
		{ID: "tenant-legacy", TenantID: 7, Name: "legacy", Type: types.ModelTypeKnowledgeQA, Source: types.ModelSourceRemote, Scope: types.ModelScopeTenant, Status: types.ModelStatusActive},
		{ID: "user-a", TenantID: 7, Name: "mine", Type: types.ModelTypeKnowledgeQA, Source: types.ModelSourceRemote, Scope: types.ModelScopeUser, OwnerUserID: "u-a", Status: types.ModelStatusActive},
		{ID: "user-b", TenantID: 7, Name: "other", Type: types.ModelTypeKnowledgeQA, Source: types.ModelSourceRemote, Scope: types.ModelScopeUser, OwnerUserID: "u-b", Status: types.ModelStatusActive},
	}
	for _, row := range rows {
		require.NoError(t, db.Create(row).Error)
	}
}

func TestModelRepositoryListSelectableModelsScopesByUsageContext(t *testing.T) {
	ctx := context.Background()
	db := setupModelScopeTestDB(t)
	seedModelScopeRows(t, db)
	repo := NewModelRepository(db)

	personal, err := repo.ListSelectable(ctx, 7, "u-a", "personal", types.ModelTypeKnowledgeQA)
	require.NoError(t, err)
	assert.ElementsMatch(t,
		[]string{"system-visible", "system-default", "tenant-legacy", "user-a"},
		modelScopeIDs(personal),
	)

	team, err := repo.ListSelectable(ctx, 7, "u-a", "team", types.ModelTypeKnowledgeQA)
	require.NoError(t, err)
	assert.ElementsMatch(t,
		[]string{"system-visible", "system-default", "tenant-legacy"},
		modelScopeIDs(team),
	)
}

func TestModelRepositoryGetByIDForUserDoesNotExposeOtherPrivateModel(t *testing.T) {
	ctx := context.Background()
	db := setupModelScopeTestDB(t)
	seedModelScopeRows(t, db)
	repo := NewModelRepository(db)

	got, err := repo.GetByIDForUser(ctx, 7, "u-a", "user-a")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "user-a", got.ID)

	other, err := repo.GetByIDForUser(ctx, 7, "u-a", "user-b")
	require.NoError(t, err)
	assert.Nil(t, other)
}
