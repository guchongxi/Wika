package intake

import (
	"context"
	"errors"

	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/gorm"
)

// GormStore 使用现有 GORM 连接读写 Wika intake 扩展表。
type GormStore struct {
	db *gorm.DB
}

// NewGormStore 创建 intake store。
func NewGormStore(db *gorm.DB) *GormStore {
	return &GormStore{db: db}
}

// GetPersonalDefaultKB 返回用户个人空间默认知识库。
func (s *GormStore) GetPersonalDefaultKB(ctx context.Context, userID string) (DefaultKB, error) {
	var mapping types.UserPersonalSpace
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&mapping).Error; err != nil {
		return DefaultKB{}, err
	}

	var defaults types.WikaSpaceDefault
	if err := s.db.WithContext(ctx).Where("tenant_id = ?", mapping.TenantID).First(&defaults).Error; err != nil {
		return DefaultKB{}, err
	}
	return DefaultKB{TenantID: mapping.TenantID, KBID: defaults.DefaultKBID}, nil
}

// FindKnowledgeIDByIdempotencyKey 查询幂等键已经创建过的知识。
func (s *GormStore) FindKnowledgeIDByIdempotencyKey(ctx context.Context, tenantID uint64, kbID, key string) (string, error) {
	if key == "" {
		return "", nil
	}
	var state types.WikaKnowledgeState
	err := s.db.WithContext(ctx).
		Where("tenant_id = ? AND kb_id = ? AND idempotency_key = ?", tenantID, kbID, key).
		First(&state).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil
		}
		return "", err
	}
	return state.KnowledgeID, nil
}

// SaveKnowledgeState 保存入库后的 Wika 扩展状态。
func (s *GormStore) SaveKnowledgeState(ctx context.Context, state *types.WikaKnowledgeState) error {
	return s.db.WithContext(ctx).Create(state).Error
}
