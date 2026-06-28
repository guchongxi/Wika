package evaluation

import (
	"context"
	"time"

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

func (s *GormStore) CreateRun(ctx context.Context, item *types.WikaEvalRun) (*types.WikaEvalRun, error) {
	if err := s.db.WithContext(ctx).Create(item).Error; err != nil {
		return nil, err
	}
	return item, nil
}

func (s *GormStore) SaveRunItems(ctx context.Context, items []*types.WikaEvalRunItem) error {
	if len(items) == 0 {
		return nil
	}
	return s.db.WithContext(ctx).Create(&items).Error
}

func (s *GormStore) CompleteRun(ctx context.Context, runID uint64, metrics RunMetrics, failed int) (*types.WikaEvalRun, error) {
	now := time.Now()
	if err := s.db.WithContext(ctx).Model(&types.WikaEvalRun{}).
		Where("id = ?", runID).
		Updates(map[string]any{
			"status":       RunStatusCompleted,
			"mrr":          metrics.MRR,
			"recall_at_5":  metrics.RecallAt5,
			"ndcg_at_5":    metrics.NDCGAt5,
			"total":        metrics.Total,
			"failed":       failed,
			"completed_at": &now,
		}).Error; err != nil {
		return nil, err
	}
	var run types.WikaEvalRun
	if err := s.db.WithContext(ctx).First(&run, "id = ?", runID).Error; err != nil {
		return nil, err
	}
	return &run, nil
}
