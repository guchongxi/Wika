package search

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
	wikagraph "github.com/Tencent/WeKnora/internal/wika/graph"
)

type graphReadService interface {
	ListEntities(ctx context.Context, input wikagraph.ListEntitiesInput) ([]*types.WikaGraphEntity, int64, error)
}

type graphSearchAdapter struct {
	service graphReadService
}

func NewGraphSearchAdapter(service graphReadService) GraphSearcher {
	if service == nil {
		return nil
	}
	return &graphSearchAdapter{service: service}
}

func (a *graphSearchAdapter) SearchGraph(ctx context.Context, scopes []ReadableScope, query string, limit int) ([]GraphContribution, error) {
	if a == nil || a.service == nil || len(scopes) == 0 || strings.TrimSpace(query) == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 5
	}
	if limit > 20 {
		limit = 20
	}
	contributions := make([]GraphContribution, 0)
	for _, scope := range scopes {
		entities, _, err := a.service.ListEntities(ctx, wikagraph.ListEntitiesInput{
			TenantID: scope.TenantID,
			KBID:     scope.KBID,
			Query:    strings.TrimSpace(query),
			Limit:    limit,
		})
		if err != nil {
			return nil, err
		}
		for _, entity := range entities {
			if entity == nil {
				continue
			}
			contributions = append(contributions, GraphContribution{
				TenantID:           entity.TenantID,
				KBID:               entity.KBID,
				EntityID:           entity.ID,
				EntityName:         entity.Name,
				EntityType:         entity.EntityType,
				SourceKnowledgeIDs: decodeKnowledgeIDs(entity.SourceKnowledgeIDs),
				Score:              entity.ConfidenceScore,
			})
		}
	}
	return contributions, nil
}

func decodeKnowledgeIDs(raw types.JSON) []string {
	if len(raw) == 0 {
		return nil
	}
	var ids []string
	if err := json.Unmarshal(raw, &ids); err != nil {
		return nil
	}
	return ids
}
