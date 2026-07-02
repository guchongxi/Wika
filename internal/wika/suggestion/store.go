package suggestion

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GormStore 使用现有 GORM 连接读写 Wika 团队推荐数据。
type GormStore struct {
	db *gorm.DB
}

// NewGormStore 创建 suggestion store。
func NewGormStore(db *gorm.DB) *GormStore {
	return &GormStore{db: db}
}

// GetKnowledge 按 ID 读取知识。
func (s *GormStore) GetKnowledge(ctx context.Context, knowledgeID string) (*types.Knowledge, error) {
	var knowledge types.Knowledge
	err := s.db.WithContext(ctx).First(&knowledge, "id = ?", knowledgeID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &knowledge, nil
}

// GetTenant 按 ID 读取空间。
func (s *GormStore) GetTenant(ctx context.Context, tenantID uint64) (*types.Tenant, error) {
	var tenant types.Tenant
	err := s.db.WithContext(ctx).First(&tenant, "id = ?", tenantID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	tenant.EnsureSpaceType()
	return &tenant, nil
}

// GetTenantMember 读取用户在空间内的成员身份。
func (s *GormStore) GetTenantMember(ctx context.Context, userID string, tenantID uint64) (*types.TenantMember, error) {
	var member types.TenantMember
	err := s.db.WithContext(ctx).
		Where("user_id = ? AND tenant_id = ?", userID, tenantID).
		First(&member).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &member, nil
}

// GetDefaultKB 返回空间默认知识库。
func (s *GormStore) GetDefaultKB(ctx context.Context, tenantID uint64) (string, error) {
	var defaults types.WikaSpaceDefault
	err := s.db.WithContext(ctx).Where("tenant_id = ?", tenantID).First(&defaults).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", ErrTargetDefaultKBNotFound
		}
		return "", err
	}
	if strings.TrimSpace(defaults.DefaultKBID) == "" {
		return "", ErrTargetDefaultKBNotFound
	}
	return defaults.DefaultKBID, nil
}

// GetSpacePolicy 读取团队推荐策略；没有策略记录时返回安全默认值。
func (s *GormStore) GetSpacePolicy(ctx context.Context, tenantID uint64) (SpacePolicy, error) {
	var policy types.WikaSpacePolicy
	err := s.db.WithContext(ctx).Where("tenant_id = ?", tenantID).First(&policy).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return SpacePolicy{AutoApplyApproved: false, PolicyVersion: 1}, nil
		}
		return SpacePolicy{}, err
	}
	version := int(policy.PolicyVersion)
	if version <= 0 {
		version = 1
	}
	return SpacePolicy{AutoApplyApproved: policy.AutoApplyApproved, PolicyVersion: version}, nil
}

// FindByIdempotencyKey 查询同一提交者对同一团队的幂等推荐。
func (s *GormStore) FindByIdempotencyKey(ctx context.Context, submitterID string, targetTenantID uint64, key string) (*SuggestionResult, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, nil
	}
	var item types.WikaKnowledgeSuggestion
	err := s.db.WithContext(ctx).
		Where("submitter_id = ? AND target_tenant_id = ? AND idempotency_key = ?", submitterID, targetTenantID, key).
		First(&item).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return resultFromSuggestion(&item, risksFromReview(item.AIReview)), nil
}

// SaveSuggestion 保存团队推荐预审结果。
func (s *GormStore) SaveSuggestion(ctx context.Context, item *types.WikaKnowledgeSuggestion) (*types.WikaKnowledgeSuggestion, error) {
	if err := s.db.WithContext(ctx).
		Omit("HumanDecision", "HumanReviewerID", "HumanComment", "ResultKnowledgeID", "AppliedAt").
		Create(item).Error; err != nil {
		return nil, err
	}
	return item, nil
}

// GetSuggestion 按 ID 读取团队推荐。
func (s *GormStore) GetSuggestion(ctx context.Context, suggestionID uint64) (*types.WikaKnowledgeSuggestion, error) {
	var item types.WikaKnowledgeSuggestion
	err := s.db.WithContext(ctx).First(&item, "id = ?", suggestionID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

// SaveHumanReview 保存人工审核和修正结果。
func (s *GormStore) SaveHumanReview(ctx context.Context, item *types.WikaKnowledgeSuggestion) (*types.WikaKnowledgeSuggestion, error) {
	if item == nil {
		return nil, errors.New("suggestion is nil")
	}
	err := s.db.WithContext(ctx).Model(&types.WikaKnowledgeSuggestion{}).
		Where("id = ?", item.ID).
		Updates(map[string]any{
			"target_kb_id":      item.TargetKBID,
			"corrected_title":   item.CorrectedTitle,
			"corrected_content": item.CorrectedContent,
			"corrected_tags":    item.CorrectedTags,
			"human_decision":    item.HumanDecision,
			"human_reviewer_id": item.HumanReviewerID,
			"human_comment":     item.HumanComment,
			"final_decision":    item.FinalDecision,
			"status":            item.Status,
			"reviewed_at":       item.ReviewedAt,
		}).Error
	if err != nil {
		return nil, err
	}
	return s.GetSuggestion(ctx, item.ID)
}

// SaveApplyResult 标记 suggestion 已应用，并写入 copy lineage。
func (s *GormStore) SaveApplyResult(ctx context.Context, suggestionID uint64, resultKnowledgeID, actorID string) (*types.WikaKnowledgeSuggestion, error) {
	var out *types.WikaKnowledgeSuggestion
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var item types.WikaKnowledgeSuggestion
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&item, "id = ?", suggestionID).Error; err != nil {
			return err
		}
		if item.Status == string(StatusApplied) && item.ResultKnowledgeID != "" {
			out = &item
			return nil
		}
		now := time.Now()
		if err := tx.Model(&types.WikaKnowledgeSuggestion{}).
			Where("id = ?", suggestionID).
			Updates(map[string]any{
				"status":              string(StatusApplied),
				"result_knowledge_id": resultKnowledgeID,
				"applied_at":          &now,
			}).Error; err != nil {
			return err
		}
		lineage := &types.WikaKnowledgeLineage{
			SourceKnowledgeID: item.SourceKnowledgeID,
			TargetKnowledgeID: resultKnowledgeID,
			SourceTenantID:    item.SourceTenantID,
			TargetTenantID:    item.TargetTenantID,
			Mode:              "copy",
			SuggestionID:      item.ID,
			CreatedBy:         actorID,
			CreatedAt:         now,
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "suggestion_id"}, {Name: "target_knowledge_id"}},
			DoNothing: true,
		}).Create(lineage).Error; err != nil {
			return err
		}
		item.Status = string(StatusApplied)
		item.ResultKnowledgeID = resultKnowledgeID
		item.AppliedAt = &now
		out = &item
		return nil
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return out, nil
}

// MarkPendingHuman 将安全门禁失败的推荐降级回人工确认。
func (s *GormStore) MarkPendingHuman(ctx context.Context, suggestionID uint64, reason string) error {
	now := time.Now()
	return s.db.WithContext(ctx).Model(&types.WikaKnowledgeSuggestion{}).
		Where("id = ?", suggestionID).
		Updates(map[string]any{
			"status":         string(StatusPendingHuman),
			"final_decision": string(DecisionNeedsConfirmation),
			"human_comment":  strings.TrimSpace(reason),
			"reviewed_at":    &now,
		}).Error
}

func risksFromReview(raw types.JSON) []string {
	if len(raw) == 0 {
		return nil
	}
	var review reviewResult
	if err := json.Unmarshal(raw, &review); err != nil {
		return nil
	}
	return review.Risks
}
