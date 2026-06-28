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
