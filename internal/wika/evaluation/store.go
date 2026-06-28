package evaluation

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/gorm"
)

// GormStore 使用 GORM 持久化 Wika 评测数据。
type GormStore struct {
	db *gorm.DB
}

func NewGormStore(db *gorm.DB) *GormStore {
	return &GormStore{db: db}
}

func (s *GormStore) CreateDataset(ctx context.Context, item *types.WikaEvalDataset) (*types.WikaEvalDataset, error) {
	if err := s.db.WithContext(ctx).Create(item).Error; err != nil {
		return nil, err
	}
	return item, nil
}

func (s *GormStore) CreateQAItem(ctx context.Context, item *types.WikaEvalQAItem) (*types.WikaEvalQAItem, error) {
	if err := s.db.WithContext(ctx).Create(item).Error; err != nil {
		return nil, err
	}
	return item, nil
}

func (s *GormStore) ListQAItems(ctx context.Context, datasetID uint64, enabledOnly bool) ([]*types.WikaEvalQAItem, error) {
	var items []*types.WikaEvalQAItem
	query := s.db.WithContext(ctx).Where("dataset_id = ?", datasetID).Order("id ASC")
	if enabledOnly {
		query = query.Where("enabled = ?", true)
	}
	if err := query.Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}
