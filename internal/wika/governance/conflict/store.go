package conflict

import (
	"context"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GormStore 持久化冲突检测任务和候选项。
type GormStore struct {
	db *gorm.DB
}

func NewGormStore(db *gorm.DB) *GormStore {
	return &GormStore{db: db}
}

func (s *GormStore) CreateCheck(ctx context.Context, check *types.WikaConflictCheck) (*types.WikaConflictCheck, error) {
	if err := s.db.WithContext(ctx).Create(check).Error; err != nil {
		return nil, err
	}
	return check, nil
}

func (s *GormStore) ListItems(ctx context.Context, input ListItemsInput) ([]*types.WikaConflictItem, int64, error) {
	limit := input.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	offset := input.Offset
	if offset < 0 {
		offset = 0
	}
	query := s.db.WithContext(ctx).Model(&types.WikaConflictItem{}).
		Where("tenant_id = ? AND kb_id = ?", input.TenantID, input.KBID)
	if input.Status != "" {
		query = query.Where("status = ?", input.Status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []*types.WikaConflictItem
	if err := query.Order("updated_at DESC, id DESC").Limit(limit).Offset(offset).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (s *GormStore) ResolveItem(ctx context.Context, input ResolveItemInput) (*types.WikaConflictItem, error) {
	if !isValidItemStatus(input.Status) {
		return nil, ErrInvalidConflictStatus
	}
	var item types.WikaConflictItem
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&item, "id = ? AND tenant_id = ?", input.ItemID, input.TenantID).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return ErrConflictItemNotFound
			}
			return err
		}
		if !canTransitionItem(item.Status, input.Status) {
			if item.Status == ItemStatusDismissed || item.Status == ItemStatusResolved {
				return ErrConflictItemTerminal
			}
			return ErrInvalidConflictStatus
		}
		updates := map[string]any{
			"status":           input.Status,
			"reviewer_comment": input.Comment,
			"resolved_by":      input.ActorID,
			"resolved_at":      input.Now,
		}
		if err := tx.Model(&types.WikaConflictItem{}).Where("id = ?", item.ID).Updates(updates).Error; err != nil {
			return err
		}
		return tx.First(&item, "id = ?", item.ID).Error
	})
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *GormStore) AcquireCheck(ctx context.Context, checkID uint64, workerID string, now time.Time, lease time.Duration) (*types.WikaConflictCheck, error) {
	if lease <= 0 {
		lease = time.Minute
	}
	lockedUntil := now.Add(lease)
	result := s.db.WithContext(ctx).Model(&types.WikaConflictCheck{}).
		Where("id = ?", checkID).
		Where("status IN ?", []string{CheckStatusPending, CheckStatusRunning, CheckStatusFailed}).
		Where("locked_until IS NULL OR locked_until < ?", now).
		Updates(map[string]any{
			"status":       CheckStatusRunning,
			"locked_by":    workerID,
			"locked_until": lockedUntil,
			"attempts":     gorm.Expr("attempts + 1"),
			"started_at":   gorm.Expr("COALESCE(started_at, ?)", now),
		})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, ErrCheckLeaseUnavailable
	}
	var check types.WikaConflictCheck
	if err := s.db.WithContext(ctx).First(&check, "id = ?", checkID).Error; err != nil {
		return nil, err
	}
	return &check, nil
}

func isValidItemStatus(status string) bool {
	switch status {
	case ItemStatusConfirmed, ItemStatusDismissed, ItemStatusResolved:
		return true
	default:
		return false
	}
}

func canTransitionItem(from, to string) bool {
	switch from {
	case ItemStatusOpen:
		return to == ItemStatusConfirmed || to == ItemStatusDismissed || to == ItemStatusResolved
	case ItemStatusConfirmed:
		return to == ItemStatusResolved
	default:
		return false
	}
}

func (s *GormStore) SaveConflictItems(ctx context.Context, check *types.WikaConflictCheck, candidates []Candidate) error {
	if check == nil || len(candidates) == 0 {
		return nil
	}
	items := make([]types.WikaConflictItem, 0, len(candidates))
	for _, candidate := range candidates {
		evidence := candidate.Evidence
		if len(evidence) == 0 {
			evidence = types.JSON([]byte(`{}`))
		}
		items = append(items, types.WikaConflictItem{
			CheckID:           check.ID,
			TenantID:          check.TenantID,
			KBID:              check.KBID,
			SourceKnowledgeID: candidate.SourceKnowledgeID,
			TargetKnowledgeID: candidate.TargetKnowledgeID,
			ConflictType:      candidate.ConflictType,
			ConfidenceScore:   candidate.ConfidenceScore,
			Evidence:          evidence,
			AIExplanation:     candidate.AIExplanation,
			Status:            ItemStatusOpen,
		})
	}
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&items).Error
}

func (s *GormStore) CompleteCheck(ctx context.Context, checkID uint64, workerID string, now time.Time) error {
	result := s.db.WithContext(ctx).Model(&types.WikaConflictCheck{}).
		Where("id = ? AND locked_by = ?", checkID, workerID).
		Updates(map[string]any{
			"status":       CheckStatusCompleted,
			"completed_at": now,
			"locked_by":    "",
			"locked_until": nil,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrCheckLeaseUnavailable
	}
	return nil
}

func (s *GormStore) FailCheck(ctx context.Context, checkID uint64, workerID string, now time.Time, errMsg string) error {
	result := s.db.WithContext(ctx).Model(&types.WikaConflictCheck{}).
		Where("id = ? AND locked_by = ?", checkID, workerID).
		Updates(map[string]any{
			"status":       CheckStatusFailed,
			"completed_at": now,
			"locked_by":    "",
			"locked_until": nil,
			"error_msg":    errMsg,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrCheckLeaseUnavailable
	}
	return nil
}
