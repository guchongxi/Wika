package orgshare

import (
	"context"
	"time"

	"github.com/Tencent/WeKnora/internal/database"
	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/gorm"
)

type Store interface {
	ListShares(ctx context.Context, input ListSharesInput) (*ListSharesResult, error)
	ValidateShareScope(ctx context.Context, orgID string, sourceTenantID uint64, sourceKBID string, targetTenantID uint64) error
	CreateShare(ctx context.Context, share *types.WikaOrgShare) (*types.WikaOrgShare, error)
	GetShare(ctx context.Context, shareID uint64) (*types.WikaOrgShare, error)
	UpdateShare(ctx context.Context, shareID uint64, updates map[string]any) (*types.WikaOrgShare, error)
}

type transactionalStore interface {
	WithTransaction(ctx context.Context, fn func(context.Context, Store) error) error
}

type GormStore struct {
	db *gorm.DB
}

func NewGormStore(db *gorm.DB) *GormStore {
	return &GormStore{db: db}
}

func (s *GormStore) WithTransaction(ctx context.Context, fn func(context.Context, Store) error) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(database.WithGormTransaction(ctx, tx), NewGormStore(tx))
	})
}

func (s *GormStore) ListShares(ctx context.Context, input ListSharesInput) (*ListSharesResult, error) {
	limit := input.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	offset := input.Offset
	if offset < 0 {
		offset = 0
	}
	query := s.db.WithContext(ctx).
		Model(&types.WikaOrgShare{}).
		Where("org_id = ?", input.OrgID).
		Where("(source_tenant_id = ? OR target_tenant_id = ?)", input.TenantID, input.TenantID)
	if input.Status != "" {
		query = query.Where("status = ?", input.Status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var items []*types.WikaOrgShare
	if err := query.
		Order("updated_at DESC, id DESC").
		Limit(limit).
		Offset(offset).
		Find(&items).Error; err != nil {
		return nil, err
	}
	return &ListSharesResult{Items: items, Total: total}, nil
}

func (s *GormStore) ValidateShareScope(ctx context.Context, orgID string, sourceTenantID uint64, sourceKBID string, targetTenantID uint64) error {
	var sourceTenant types.Tenant
	if err := s.db.WithContext(ctx).First(&sourceTenant, "id = ? AND space_type = ?", sourceTenantID, types.SpaceTypeTeam).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return ErrScopeDenied
		}
		return err
	}
	var targetTenant types.Tenant
	if err := s.db.WithContext(ctx).First(&targetTenant, "id = ? AND space_type = ?", targetTenantID, types.SpaceTypeTeam).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return ErrScopeDenied
		}
		return err
	}
	var sourceKB types.KnowledgeBase
	if err := s.db.WithContext(ctx).
		First(&sourceKB, "id = ? AND tenant_id = ? AND type = ?", sourceKBID, sourceTenantID, types.KnowledgeBaseTypeDocument).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return ErrScopeDenied
		}
		return err
	}
	var count int64
	if err := s.db.WithContext(ctx).
		Table("organizations AS o").
		Joins("JOIN organization_tenant_members AS source_otm ON source_otm.organization_id = o.id AND source_otm.tenant_id = ?", sourceTenantID).
		Joins("JOIN organization_tenant_members AS target_otm ON target_otm.organization_id = o.id AND target_otm.tenant_id = ?", targetTenantID).
		Where("o.id = ?", orgID).
		Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return ErrScopeDenied
	}
	return nil
}

func (s *GormStore) CreateShare(ctx context.Context, share *types.WikaOrgShare) (*types.WikaOrgShare, error) {
	if err := s.db.WithContext(ctx).Create(share).Error; err != nil {
		return nil, err
	}
	return share, nil
}

func (s *GormStore) GetShare(ctx context.Context, shareID uint64) (*types.WikaOrgShare, error) {
	var share types.WikaOrgShare
	if err := s.db.WithContext(ctx).First(&share, "id = ?", shareID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrShareNotFound
		}
		return nil, err
	}
	return &share, nil
}

func (s *GormStore) UpdateShare(ctx context.Context, shareID uint64, updates map[string]any) (*types.WikaOrgShare, error) {
	if _, ok := updates["updated_at"]; !ok {
		updates["updated_at"] = time.Now()
	}
	result := s.db.WithContext(ctx).Model(&types.WikaOrgShare{}).
		Where("id = ?", shareID).
		Updates(updates)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, ErrShareNotFound
	}
	return s.GetShare(ctx, shareID)
}
