package space

import (
	"context"
	"errors"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// GormStore 使用现有 GORM 连接持久化 Wika 空间数据。
type GormStore struct {
	db *gorm.DB
}

// EnsureTeamDefaults 在团队空间创建默认知识库和默认关闭的团队推荐策略。
func (s *GormStore) EnsureTeamDefaults(ctx context.Context, userID string, tenant *types.Tenant) error {
	if tenant == nil || tenant.ID == 0 {
		return errors.New("team tenant is required")
	}
	tenant.EnsureSpaceType()
	if tenant.SpaceType == types.SpaceTypePersonal {
		return nil
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		var defaults types.WikaSpaceDefault
		err := tx.Where("tenant_id = ?", tenant.ID).First(&defaults).Error
		switch {
		case err == nil:
			return ensureDefaultPolicy(tx, tenant.ID, now)
		case !errors.Is(err, gorm.ErrRecordNotFound):
			return err
		}

		defaultKB := &types.KnowledgeBase{
			ID:          uuid.New().String(),
			Name:        "团队知识",
			Description: "团队空间默认知识库",
			TenantID:    tenant.ID,
			CreatorID:   userID,
			Type:        types.KnowledgeBaseTypeDocument,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		defaultKB.EnsureDefaults()
		defaultKB.Normalize()
		if err := tx.Create(defaultKB).Error; err != nil {
			return err
		}

		defaults = types.WikaSpaceDefault{
			TenantID:    tenant.ID,
			DefaultKBID: defaultKB.ID,
			CreatedBy:   userID,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := tx.Create(&defaults).Error; err != nil {
			return err
		}
		return ensureDefaultPolicy(tx, tenant.ID, now)
	})
}

func ensureDefaultPolicy(tx *gorm.DB, tenantID uint64, now time.Time) error {
	var policy types.WikaSpacePolicy
	err := tx.Where("tenant_id = ?", tenantID).First(&policy).Error
	switch {
	case err == nil:
		return nil
	case !errors.Is(err, gorm.ErrRecordNotFound):
		return err
	}
	if tenantID == 0 {
		return errors.New("tenant id is required")
	}
	return tx.Create(&types.WikaSpacePolicy{
		TenantID:          tenantID,
		AutoApplyApproved: false,
		PolicyVersion:     1,
		SafetyPolicy:      types.JSON([]byte("{}")),
		CreatedAt:         now,
		UpdatedAt:         now,
	}).Error
}

// NewGormStore 创建 GORM 版本的个人空间存储。
func NewGormStore(db *gorm.DB) *GormStore {
	return &GormStore{db: db}
}

// GetPersonalSpace 根据 user_personal_spaces 映射读取个人空间。
func (s *GormStore) GetPersonalSpace(ctx context.Context, userID string) (*types.Tenant, error) {
	var mapping types.UserPersonalSpace
	err := s.db.WithContext(ctx).
		Where("user_id = ?", userID).
		First(&mapping).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPersonalSpaceNotFound
		}
		return nil, err
	}

	var tenant types.Tenant
	err = s.db.WithContext(ctx).
		Where("id = ? AND space_type = ?", mapping.TenantID, types.SpaceTypePersonal).
		First(&tenant).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPersonalSpaceNotFound
		}
		return nil, err
	}
	return &tenant, nil
}

// CreatePersonalSpace 在一个事务中创建 personal tenant、Owner 成员和用户映射。
func (s *GormStore) CreatePersonalSpace(ctx context.Context, userID string, tenant *types.Tenant, member *types.TenantMember) (*types.Tenant, error) {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		tenant.SpaceType = types.SpaceTypePersonal
		tenant.EnsureSpaceType()
		if err := tx.Create(tenant).Error; err != nil {
			return err
		}

		member.UserID = userID
		member.TenantID = tenant.ID
		if member.Role == "" {
			member.Role = types.TenantRoleOwner
		}
		if member.Status == "" {
			member.Status = types.TenantMemberStatusActive
		}
		if err := tx.Create(member).Error; err != nil {
			return err
		}

		mapping := &types.UserPersonalSpace{
			UserID:   userID,
			TenantID: tenant.ID,
		}
		if err := tx.Create(mapping).Error; err != nil {
			return err
		}

		defaultKB := &types.KnowledgeBase{
			ID:          uuid.New().String(),
			Name:        "个人知识",
			Description: "个人空间默认知识库",
			TenantID:    tenant.ID,
			CreatorID:   userID,
			Type:        types.KnowledgeBaseTypeDocument,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		defaultKB.EnsureDefaults()
		defaultKB.Normalize()
		if err := tx.Create(defaultKB).Error; err != nil {
			return err
		}

		defaults := &types.WikaSpaceDefault{
			TenantID:    tenant.ID,
			DefaultKBID: defaultKB.ID,
			CreatedBy:   userID,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := tx.Create(defaults).Error; err != nil {
			return err
		}

		policy := &types.WikaSpacePolicy{
			TenantID:          tenant.ID,
			AutoApplyApproved: false,
			PolicyVersion:     1,
			SafetyPolicy:      types.JSON([]byte("{}")),
			CreatedAt:         now,
			UpdatedAt:         now,
		}
		return tx.Create(policy).Error
	})
	if err != nil {
		return nil, err
	}
	return tenant, nil
}
