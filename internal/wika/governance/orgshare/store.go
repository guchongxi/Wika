package orgshare

import (
	"context"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/gorm"
)

type Store interface {
	CreateShare(ctx context.Context, share *types.WikaOrgShare) (*types.WikaOrgShare, error)
	GetShare(ctx context.Context, shareID uint64) (*types.WikaOrgShare, error)
	UpdateShare(ctx context.Context, shareID uint64, updates map[string]any) (*types.WikaOrgShare, error)
}

type GormStore struct {
	db *gorm.DB
}

func NewGormStore(db *gorm.DB) *GormStore {
	return &GormStore{db: db}
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
