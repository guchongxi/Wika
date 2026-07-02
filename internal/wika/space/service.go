package space

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

// ErrPersonalSpaceNotFound 表示用户还没有个人空间映射。
var ErrPersonalSpaceNotFound = errors.New("personal space not found")

// PersonalSpaceStore 隔离个人空间创建所需的持久化操作。
type PersonalSpaceStore interface {
	GetPersonalSpace(ctx context.Context, userID string) (*types.Tenant, error)
	CreatePersonalSpace(ctx context.Context, userID string, tenant *types.Tenant, member *types.TenantMember) (*types.Tenant, error)
	EnsureTeamDefaults(ctx context.Context, userID string, tenant *types.Tenant) error
}

// Service 编排 Wika 空间行为，不直接暴露 Tenant 的底层实现细节。
type Service struct {
	store PersonalSpaceStore
}

// NewService 创建空间服务。
func NewService(store PersonalSpaceStore) *Service {
	return &Service{store: store}
}

// GetOrCreatePersonalSpace 返回用户个人空间；不存在时创建 personal tenant 和 Owner 成员。
func (s *Service) GetOrCreatePersonalSpace(ctx context.Context, userID, displayName string) (*types.Tenant, error) {
	tenant, err := s.store.GetPersonalSpace(ctx, userID)
	if err == nil {
		return tenant, nil
	}
	if !errors.Is(err, ErrPersonalSpaceNotFound) {
		return nil, err
	}

	now := time.Now()
	tenant = &types.Tenant{
		Name:      personalSpaceName(displayName),
		Status:    "active",
		SpaceType: types.SpaceTypePersonal,
		CreatedAt: now,
		UpdatedAt: now,
	}
	member := &types.TenantMember{
		UserID:   userID,
		Role:     types.TenantRoleOwner,
		Status:   types.TenantMemberStatusActive,
		JoinedAt: now,
	}
	return s.store.CreatePersonalSpace(ctx, userID, tenant, member)
}

// EnsureTeamDefaults 为团队空间补齐 Wika 默认知识库和安全默认策略。
func (s *Service) EnsureTeamDefaults(ctx context.Context, userID string, tenant *types.Tenant) error {
	if s == nil || s.store == nil {
		return nil
	}
	return s.store.EnsureTeamDefaults(ctx, userID, tenant)
}

func personalSpaceName(displayName string) string {
	name := strings.TrimSpace(displayName)
	if name == "" {
		return "个人空间"
	}
	return fmt.Sprintf("%s的个人空间", name)
}
