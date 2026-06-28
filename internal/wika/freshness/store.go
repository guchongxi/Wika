package freshness

import (
	"context"
	"errors"

	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

func (s *GormStore) ListChecks(ctx context.Context, tenantID uint64, kbID string) ([]*types.WikaFreshnessCheck, error) {
	var checks []*types.WikaFreshnessCheck
	err := s.db.WithContext(ctx).
		Where("tenant_id = ? AND kb_id = ?", tenantID, kbID).
		Order("checked_at DESC, id DESC").
		Find(&checks).Error
	return checks, err
}

func (s *GormStore) ListItems(ctx context.Context, tenantID uint64, kbID, status string) ([]*types.WikaFreshnessCheckItem, error) {
	var items []*types.WikaFreshnessCheckItem
	query := s.db.WithContext(ctx).
		Where("tenant_id = ? AND kb_id = ?", tenantID, kbID).
		Order("created_at DESC, id DESC")
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if err := query.Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (s *GormStore) HandleItem(ctx context.Context, update ItemUpdate) (*types.WikaFreshnessCheckItem, error) {
	var item types.WikaFreshnessCheckItem
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&item, "id = ? AND tenant_id = ?", update.ItemID, update.TenantID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrFreshnessItemNotFound
			}
			return err
		}
		previousStatus := item.Status
		itemUpdates := map[string]any{
			"status":            update.Status,
			"resolution_action": update.Action,
			"resolution_note":   update.Note,
			"previous_status":   previousStatus,
			"resolved_by":       update.ActorID,
			"resolved_at":       update.Now,
		}
		if err := tx.Model(&types.WikaFreshnessCheckItem{}).
			Where("id = ?", item.ID).
			Updates(itemUpdates).Error; err != nil {
			return err
		}
		stateUpdates := map[string]any{}
		if update.KnowledgeFreshnessStatus != "" {
			stateUpdates["freshness_status"] = update.KnowledgeFreshnessStatus
		}
		if update.KnowledgeReviewStatus != "" {
			stateUpdates["review_status"] = update.KnowledgeReviewStatus
		}
		if update.ExpiresAt != nil {
			stateUpdates["expires_at"] = update.ExpiresAt
		}
		if len(stateUpdates) > 0 {
			if err := tx.Model(&types.WikaKnowledgeState{}).
				Where("knowledge_id = ? AND tenant_id = ?", item.KnowledgeID, update.TenantID).
				Updates(stateUpdates).Error; err != nil {
				return err
			}
		}
		return tx.First(&item, "id = ?", item.ID).Error
	}); err != nil {
		return nil, err
	}
	return &item, nil
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
