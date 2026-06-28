package graph

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

type fakeGraphStore struct {
	entities []*types.WikaGraphEntity
	edges    []*types.WikaGraphEdge
}

func (s *fakeGraphStore) Overview(ctx context.Context, tenantID uint64, kbID string) (*Overview, error) {
	return &Overview{TenantID: tenantID, KBID: kbID, EntityCount: int64(len(s.entities)), EdgeCount: int64(len(s.edges))}, nil
}

func (s *fakeGraphStore) ListEntities(ctx context.Context, input ListEntitiesInput) ([]*types.WikaGraphEntity, int64, error) {
	return s.entities, int64(len(s.entities)), nil
}

func (s *fakeGraphStore) GetEntity(ctx context.Context, tenantID uint64, kbID string, entityID uint64) (*types.WikaGraphEntity, error) {
	for _, entity := range s.entities {
		if entity.ID == entityID && entity.TenantID == tenantID && entity.KBID == kbID {
			return entity, nil
		}
	}
	return nil, ErrGraphEntityNotFound
}

func (s *fakeGraphStore) ListEdges(ctx context.Context, input ListEdgesInput) ([]*types.WikaGraphEdge, int64, error) {
	return s.edges, int64(len(s.edges)), nil
}

func TestGraphServiceHidesPersonalEvidenceForSystemAdmin(t *testing.T) {
	store := &fakeGraphStore{
		entities: []*types.WikaGraphEntity{
			{ID: 1, TenantID: 80, KBID: "kb-personal", Name: "索引延迟", EntityType: "concept", Summary: "个人排查细节"},
		},
		edges: []*types.WikaGraphEdge{
			{ID: 11, TenantID: 80, KBID: "kb-personal", SourceEntityID: 1, TargetEntityID: 2, RelationType: "causes", EvidenceText: "包含个人证据正文"},
		},
	}
	svc := &Service{store: store}

	entities, _, err := svc.ListEntities(context.Background(), ListEntitiesInput{
		TenantID:      80,
		KBID:          "kb-personal",
		SystemAdmin:   true,
		PersonalScope: true,
	})
	if err != nil {
		t.Fatalf("ListEntities returned error: %v", err)
	}
	if len(entities) != 1 || entities[0].Summary != "" {
		t.Fatalf("expected system admin personal entity summary to be hidden: %+v", entities)
	}

	edges, _, err := svc.ListEdges(context.Background(), ListEdgesInput{
		TenantID:      80,
		KBID:          "kb-personal",
		SystemAdmin:   true,
		PersonalScope: true,
	})
	if err != nil {
		t.Fatalf("ListEdges returned error: %v", err)
	}
	if len(edges) != 1 || edges[0].EvidenceText != "" {
		t.Fatalf("expected system admin personal edge evidence to be hidden: %+v", edges)
	}
}

func TestGraphServiceKeepsTeamEvidenceForRegularUser(t *testing.T) {
	store := &fakeGraphStore{
		entities: []*types.WikaGraphEntity{
			{ID: 1, TenantID: 80, KBID: "kb-team", Name: "索引延迟", EntityType: "concept", Summary: "团队排查细节"},
		},
		edges: []*types.WikaGraphEdge{
			{ID: 11, TenantID: 80, KBID: "kb-team", SourceEntityID: 1, TargetEntityID: 2, RelationType: "causes", EvidenceText: "团队证据正文"},
		},
	}
	svc := &Service{store: store}

	entities, _, err := svc.ListEntities(context.Background(), ListEntitiesInput{TenantID: 80, KBID: "kb-team"})
	if err != nil {
		t.Fatalf("ListEntities returned error: %v", err)
	}
	if len(entities) != 1 || entities[0].Summary != "团队排查细节" {
		t.Fatalf("expected regular user to see team summary: %+v", entities)
	}

	edges, _, err := svc.ListEdges(context.Background(), ListEdgesInput{TenantID: 80, KBID: "kb-team"})
	if err != nil {
		t.Fatalf("ListEdges returned error: %v", err)
	}
	if len(edges) != 1 || edges[0].EvidenceText != "团队证据正文" {
		t.Fatalf("expected regular user to see team evidence: %+v", edges)
	}
}
