package auth

import (
	"context"
	"errors"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/gorm"
)

// GormTokenStore 使用 GORM 持久化用户级 MCP token。
type GormTokenStore struct {
	db *gorm.DB
}

// NewGormTokenStore 创建 GORM token 存储。
func NewGormTokenStore(db *gorm.DB) *GormTokenStore {
	return &GormTokenStore{db: db}
}

// SaveToken 保存 token 元数据，不包含明文 token。
func (s *GormTokenStore) SaveToken(ctx context.Context, token *types.WikaUserToken) error {
	return s.db.WithContext(ctx).Create(token).Error
}

// FindTokenByHash 按 hash 读取 token 元数据。
func (s *GormTokenStore) FindTokenByHash(ctx context.Context, tokenHash string) (*types.WikaUserToken, error) {
	var token types.WikaUserToken
	err := s.db.WithContext(ctx).
		Where("token_hash = ?", tokenHash).
		First(&token).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTokenInvalid
		}
		return nil, err
	}
	return &token, nil
}

// ListTokens 列出用户在指定默认空间下创建的 token。
func (s *GormTokenStore) ListTokens(ctx context.Context, userID string, tenantID uint64) ([]*types.WikaUserToken, error) {
	var tokens []*types.WikaUserToken
	err := s.db.WithContext(ctx).
		Where("user_id = ? AND tenant_id = ?", userID, tenantID).
		Order("created_at DESC, id DESC").
		Find(&tokens).Error
	return tokens, err
}

// RevokeToken 设置 revoked_at；只允许撤销用户自己的 token。
func (s *GormTokenStore) RevokeToken(ctx context.Context, userID string, tokenID uint64, revokedAt time.Time) error {
	res := s.db.WithContext(ctx).
		Model(&types.WikaUserToken{}).
		Where("id = ? AND user_id = ? AND revoked_at IS NULL", tokenID, userID).
		Update("revoked_at", revokedAt)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrTokenInvalid
	}
	return nil
}
