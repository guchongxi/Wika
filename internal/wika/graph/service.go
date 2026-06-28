package graph

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

type Store interface {
	Overview(ctx context.Context, tenantID uint64, kbID string) (*Overview, error)
	ListEntities(ctx context.Context, input ListEntitiesInput) ([]*types.WikaGraphEntity, int64, error)
	GetEntity(ctx context.Context, tenantID uint64, kbID string, entityID uint64) (*types.WikaGraphEntity, error)
	ListEdges(ctx context.Context, input ListEdgesInput) ([]*types.WikaGraphEdge, int64, error)
}

// Service 编排图谱读模型查询和字段级裁剪。
type Service struct {
	store Store
}

func NewService(store *GormStore) *Service {
	return &Service{store: store}
}

func (s *Service) Overview(ctx context.Context, input OverviewInput) (*Overview, error) {
	return s.store.Overview(ctx, input.TenantID, input.KBID)
}

func (s *Service) ListEntities(ctx context.Context, input ListEntitiesInput) ([]*types.WikaGraphEntity, int64, error) {
	entities, total, err := s.store.ListEntities(ctx, input)
	if err != nil {
		return nil, 0, err
	}
	if shouldHidePersonalEvidence(input.SystemAdmin, input.PersonalScope) {
		for _, entity := range entities {
			if entity != nil {
				entity.Summary = ""
			}
		}
	}
	return entities, total, nil
}

func (s *Service) GetEntity(ctx context.Context, input GetEntityInput) (*types.WikaGraphEntity, error) {
	entity, err := s.store.GetEntity(ctx, input.TenantID, input.KBID, input.EntityID)
	if err != nil {
		return nil, err
	}
	if shouldHidePersonalEvidence(input.SystemAdmin, input.PersonalScope) {
		entity.Summary = ""
	}
	return entity, nil
}

func (s *Service) ListEdges(ctx context.Context, input ListEdgesInput) ([]*types.WikaGraphEdge, int64, error) {
	edges, total, err := s.store.ListEdges(ctx, input)
	if err != nil {
		return nil, 0, err
	}
	if shouldHidePersonalEvidence(input.SystemAdmin, input.PersonalScope) {
		for _, edge := range edges {
			if edge != nil {
				edge.EvidenceText = ""
			}
		}
	}
	return edges, total, nil
}

func shouldHidePersonalEvidence(systemAdmin, personalScope bool) bool {
	return systemAdmin
}
