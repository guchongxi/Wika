package scope

import (
	"context"
	"errors"

	"github.com/Tencent/WeKnora/internal/types"
)

var ErrResourceNotFound = errors.New("resource not found")

type Action string

const (
	ActionRead         Action = "read"
	ActionMetadataRead Action = "metadata_read"
)

type ResourceKind string

const (
	ResourceKnowledgeBase ResourceKind = "kb"
)

type ScopeSource string

const (
	ScopeSourcePersonal ScopeSource = "personal"
	ScopeSourceTeam     ScopeSource = "team"
	ScopeSourceShared   ScopeSource = "shared"
)

type Actor struct {
	UserID        string
	IsSystemAdmin bool
}

type Resource struct {
	Kind ResourceKind
	ID   string
}

type Scope struct {
	TenantID      uint64
	KBID          string
	Source        ScopeSource
	Role          types.TenantRole
	AllowedFields []string
}

type Decision struct {
	Allowed      bool
	NotFound     bool
	MetadataOnly bool
	Scopes       []Scope
}

type Store interface {
	GetKnowledgeBase(ctx context.Context, kbID string) (*types.KnowledgeBase, error)
	GetTenant(ctx context.Context, tenantID uint64) (*types.Tenant, error)
	GetTenantMember(ctx context.Context, userID string, tenantID uint64) (*types.TenantMember, error)
	ListSharedKnowledgeBaseScopes(ctx context.Context, userID string, kbID string) ([]Scope, error)
}

type Resolver struct {
	store Store
}

func NewResolver(store Store) *Resolver {
	return &Resolver{store: store}
}

func (r *Resolver) Resolve(ctx context.Context, actor Actor, resource Resource, action Action) (Decision, error) {
	switch resource.Kind {
	case ResourceKnowledgeBase:
		return r.resolveKnowledgeBase(ctx, actor, resource.ID, action)
	default:
		return Decision{NotFound: true}, nil
	}
}

func (r *Resolver) resolveKnowledgeBase(ctx context.Context, actor Actor, kbID string, action Action) (Decision, error) {
	kb, err := r.store.GetKnowledgeBase(ctx, kbID)
	if err != nil {
		if errors.Is(err, ErrResourceNotFound) {
			return Decision{NotFound: true}, nil
		}
		return Decision{}, err
	}
	tenant, err := r.store.GetTenant(ctx, kb.TenantID)
	if err != nil {
		if errors.Is(err, ErrResourceNotFound) {
			return Decision{NotFound: true}, nil
		}
		return Decision{}, err
	}

	source := ScopeSourceTeam
	if tenant.SpaceType == types.SpaceTypePersonal {
		source = ScopeSourcePersonal
	}

	if actor.IsSystemAdmin && action == ActionMetadataRead {
		return Decision{
			Allowed:      true,
			MetadataOnly: true,
			Scopes: []Scope{{
				TenantID:      tenant.ID,
				KBID:          kb.ID,
				Source:        source,
				AllowedFields: metadataAllowedFields(),
			}},
		}, nil
	}

	member, err := r.store.GetTenantMember(ctx, actor.UserID, tenant.ID)
	if err != nil {
		if errors.Is(err, ErrResourceNotFound) {
			if tenant.SpaceType != types.SpaceTypePersonal {
				sharedScopes, sharedErr := r.store.ListSharedKnowledgeBaseScopes(ctx, actor.UserID, kb.ID)
				if sharedErr != nil {
					return Decision{}, sharedErr
				}
				if len(sharedScopes) > 0 {
					return Decision{Allowed: true, Scopes: sharedScopes}, nil
				}
			}
			return hiddenDecisionFor(tenant), nil
		}
		return Decision{}, err
	}
	if tenant.SpaceType == types.SpaceTypePersonal && member.Role != types.TenantRoleOwner {
		return Decision{NotFound: true}, nil
	}
	return Decision{
		Allowed: true,
		Scopes: []Scope{{
			TenantID: tenant.ID,
			KBID:     kb.ID,
			Source:   source,
			Role:     member.Role,
		}},
	}, nil
}

func hiddenDecisionFor(tenant *types.Tenant) Decision {
	if tenant != nil && tenant.SpaceType == types.SpaceTypePersonal {
		return Decision{NotFound: true}
	}
	return Decision{}
}

func metadataAllowedFields() []string {
	return []string{"id", "tenant_id", "name", "type", "status", "created_at", "updated_at"}
}
