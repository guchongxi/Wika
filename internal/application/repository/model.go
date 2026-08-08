package repository

import (
	"context"
	"errors"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

// modelRepository implements the model repository interface
type modelRepository struct {
	db *gorm.DB
}

// NewModelRepository creates a new model repository
func NewModelRepository(db *gorm.DB) interfaces.ModelRepository {
	return &modelRepository{db: db}
}

// Create creates a new model
func (r *modelRepository) Create(ctx context.Context, m *types.Model) error {
	return r.db.WithContext(ctx).Create(m).Error
}

// GetByID retrieves a model by ID
func (r *modelRepository) GetByID(ctx context.Context, tenantID uint64, id string) (*types.Model, error) {
	var m types.Model
	if err := r.db.WithContext(ctx).
		Where("id = ?", id).
		Where(
			"(is_builtin = ? OR scope = ? OR (tenant_id = ? AND (scope = ? OR scope = '' OR scope IS NULL)))",
			true, types.ModelScopeSystem, tenantID, types.ModelScopeTenant,
		).
		First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &m, nil
}

func (r *modelRepository) GetByIDForUser(
	ctx context.Context, tenantID uint64, userID, id string,
) (*types.Model, error) {
	var m types.Model
	if err := r.db.WithContext(ctx).
		Where("id = ?", id).
		Where(
			`(is_builtin = ? OR scope = ?
				OR (tenant_id = ? AND (scope = ? OR scope = '' OR scope IS NULL))
				OR (tenant_id = ? AND scope = ? AND owner_user_id = ?))`,
			true, types.ModelScopeSystem,
			tenantID, types.ModelScopeTenant,
			tenantID, types.ModelScopeUser, userID,
		).
		First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &m, nil
}

func (r *modelRepository) GetSystemByID(ctx context.Context, id string) (*types.Model, error) {
	var m types.Model
	if err := r.db.WithContext(ctx).
		Where("id = ?", id).
		Where("(is_builtin = ? OR scope = ?)", true, types.ModelScopeSystem).
		First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &m, nil
}

// List lists models with optional filtering
func (r *modelRepository) List(
	ctx context.Context, tenantID uint64, modelType types.ModelType, source types.ModelSource,
) ([]*types.Model, error) {
	var models []*types.Model
	query := r.db.WithContext(ctx).Where(
		"(is_builtin = ? OR scope = ? OR (tenant_id = ? AND (scope = ? OR scope = '' OR scope IS NULL)))",
		true, types.ModelScopeSystem, tenantID, types.ModelScopeTenant,
	)

	if modelType != "" {
		query = query.Where("type = ?", modelType)
	}

	if source != "" {
		query = query.Where("source = ?", source)
	}

	if err := query.Find(&models).Error; err != nil {
		return nil, err
	}

	return models, nil
}

func (r *modelRepository) ListSystem(ctx context.Context, modelType types.ModelType) ([]*types.Model, error) {
	var models []*types.Model
	query := r.db.WithContext(ctx).
		Where("(is_builtin = ? OR scope = ?)", true, types.ModelScopeSystem)
	if modelType != "" {
		query = query.Where("type = ?", modelType)
	}
	if err := query.Order("type ASC, name ASC").Find(&models).Error; err != nil {
		return nil, err
	}
	return models, nil
}

func (r *modelRepository) ListByOwner(
	ctx context.Context, userID string, modelType types.ModelType,
) ([]*types.Model, error) {
	var models []*types.Model
	query := r.db.WithContext(ctx).
		Where("scope = ? AND owner_user_id = ?", types.ModelScopeUser, userID)
	if modelType != "" {
		query = query.Where("type = ?", modelType)
	}
	if err := query.Order("type ASC, name ASC").Find(&models).Error; err != nil {
		return nil, err
	}
	return models, nil
}

func (r *modelRepository) ListSelectable(
	ctx context.Context,
	tenantID uint64,
	userID string,
	usageContext types.ModelUsageContext,
	modelType types.ModelType,
) ([]*types.Model, error) {
	var models []*types.Model
	query := r.db.WithContext(ctx).Where(
		`((is_builtin = ? OR scope = ?) AND (user_selectable = ? OR is_default = ?))
			OR (tenant_id = ? AND (scope = ? OR scope = '' OR scope IS NULL))`,
		true, types.ModelScopeSystem, true, true,
		tenantID, types.ModelScopeTenant,
	)
	if usageContext == types.ModelUsageContextPersonal && userID != "" {
		query = query.Or(
			"tenant_id = ? AND scope = ? AND owner_user_id = ?",
			tenantID, types.ModelScopeUser, userID,
		)
	}
	if modelType != "" {
		query = query.Where("type = ?", modelType)
	}
	if err := query.Order("type ASC, name ASC").Find(&models).Error; err != nil {
		return nil, err
	}
	return models, nil
}

// Update updates a model
func (r *modelRepository) Update(ctx context.Context, m *types.Model) error {
	// Use Select to explicitly update all fields, including zero values like false
	return r.db.WithContext(ctx).Debug().Model(&types.Model{}).Where(
		"id = ? AND tenant_id = ?", m.ID, m.TenantID,
	).Select("*").Updates(m).Error
}

// Delete deletes a model
func (r *modelRepository) Delete(ctx context.Context, tenantID uint64, id string) error {
	return r.db.WithContext(ctx).Where(
		"id = ? AND tenant_id = ?", id, tenantID,
	).Delete(&types.Model{}).Error
}

// ClearDefaultByType clears the default flag for all models of a specific type
// This is a batch operation that updates all matching records in one query
func (r *modelRepository) ClearDefaultByType(
	ctx context.Context,
	tenantID uint,
	modelType types.ModelType,
	excludeID string,
) error {
	query := r.db.WithContext(ctx).Model(&types.Model{}).Where(
		"tenant_id = ? AND type = ? AND is_default = ?", tenantID, modelType, true,
	)

	// If excludeID is provided, exclude that model from the update
	if excludeID != "" {
		query = query.Where("id != ?", excludeID)
	}

	// Batch update: set is_default to false for all matching records
	return query.Update("is_default", false).Error
}

func (r *modelRepository) ClearSystemDefaultByType(
	ctx context.Context,
	modelType types.ModelType,
	excludeID string,
) error {
	query := r.db.WithContext(ctx).Model(&types.Model{}).
		Where("(is_builtin = ? OR scope = ?) AND type = ? AND is_default = ?",
			true, types.ModelScopeSystem, modelType, true)

	if excludeID != "" {
		query = query.Where("id != ?", excludeID)
	}

	return query.Update("is_default", false).Error
}
