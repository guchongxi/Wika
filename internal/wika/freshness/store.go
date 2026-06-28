package freshness

import (
	"context"
	"errors"

	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/gorm"
)

// GormStore 使用 GORM 持久化保鲜扫描结果。
type GormStore struct {
	db *gorm.DB
}

func NewGormStore(db *gorm.DB) *GormStore {
	return &GormStore{db: db}
}

func (s *GormStore) ListKnowledgeStates(ctx context.Context, tenantID uint64, kbID string) ([]*types.WikaKnowledgeState, error) {
	var states []*types.WikaKnowledgeState
	err := s.db.WithContext(ctx).
		Where("tenant_id = ? AND kb_id = ?", tenantID, kbID).
		Order("knowledge_id ASC").
		Find(&states).Error
	return states, err
}

func (s *GormStore) LastAccess(ctx context.Context, knowledgeID string) (*types.KnowledgeAccessDaily, error) {
	var access types.KnowledgeAccessDaily
	err := s.db.WithContext(ctx).
		Where("knowledge_id = ?", knowledgeID).
		Order("last_accessed_at DESC").
		First(&access).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &access, nil
}

func (s *GormStore) SaveCheck(ctx context.Context, check *types.WikaFreshnessCheck, items []*types.WikaFreshnessCheckItem) (*types.WikaFreshnessCheck, error) {
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(check).Error; err != nil {
			return err
		}
		for _, item := range items {
			if item == nil {
				continue
			}
			item.CheckID = check.ID
			if err := tx.Create(item).Error; err != nil {
				return err
			}
			if err := tx.Model(&types.WikaKnowledgeState{}).
				Where("knowledge_id = ?", item.KnowledgeID).
				Update("freshness_status", stateStatusForIssue(item.IssueType)).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return check, nil
}

func stateStatusForIssue(issueType string) string {
	switch issueType {
	case IssueExpired:
		return "expired"
	case IssueExpiring:
		return "expiring"
	case IssueStale:
		return "stale"
	case IssueLowQuality:
		return "low_quality"
	default:
		return "needs_review"
	}
}
