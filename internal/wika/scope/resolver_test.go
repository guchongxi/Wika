package scope

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

type fakeScopeStore struct {
	tenants map[uint64]*types.Tenant
	kbs     map[string]*types.KnowledgeBase
	members map[string]*types.TenantMember
}

func (f *fakeScopeStore) GetKnowledgeBase(ctx context.Context, kbID string) (*types.KnowledgeBase, error) {
	if kb, ok := f.kbs[kbID]; ok {
		return kb, nil
	}
	return nil, ErrResourceNotFound
}

func (f *fakeScopeStore) GetTenant(ctx context.Context, tenantID uint64) (*types.Tenant, error) {
	if tenant, ok := f.tenants[tenantID]; ok {
		return tenant, nil
	}
	return nil, ErrResourceNotFound
}

func (f *fakeScopeStore) GetTenantMember(ctx context.Context, userID string, tenantID uint64) (*types.TenantMember, error) {
	key := userID + ":"
	if tenantID == 1 {
		key += "1"
	} else {
		key += "2"
	}
	if member, ok := f.members[key]; ok {
		return member, nil
	}
	return nil, ErrResourceNotFound
}

func TestResolveKnowledgeBaseReadAllowsPersonalOwner(t *testing.T) {
	resolver := NewResolver(&fakeScopeStore{
		tenants: map[uint64]*types.Tenant{1: {ID: 1, SpaceType: types.SpaceTypePersonal}},
		kbs:     map[string]*types.KnowledgeBase{"kb-1": {ID: "kb-1", TenantID: 1}},
		members: map[string]*types.TenantMember{"user-1:1": {UserID: "user-1", TenantID: 1, Role: types.TenantRoleOwner}},
	})

	decision, err := resolver.Resolve(context.Background(), Actor{UserID: "user-1"}, Resource{Kind: ResourceKnowledgeBase, ID: "kb-1"}, ActionRead)

	if err != nil {
		t.Fatalf("expected allow, got error: %v", err)
	}
	if !decision.Allowed || decision.NotFound || decision.MetadataOnly {
		t.Fatalf("expected full allow, got %+v", decision)
	}
	if len(decision.Scopes) != 1 || decision.Scopes[0].Source != ScopeSourcePersonal {
		t.Fatalf("expected personal scope, got %+v", decision.Scopes)
	}
}

func TestResolveKnowledgeBaseReadHidesPersonalSpaceFromNonOwner(t *testing.T) {
	resolver := NewResolver(&fakeScopeStore{
		tenants: map[uint64]*types.Tenant{1: {ID: 1, SpaceType: types.SpaceTypePersonal}},
		kbs:     map[string]*types.KnowledgeBase{"kb-1": {ID: "kb-1", TenantID: 1}},
		members: map[string]*types.TenantMember{},
	})

	decision, err := resolver.Resolve(context.Background(), Actor{UserID: "user-2"}, Resource{Kind: ResourceKnowledgeBase, ID: "kb-1"}, ActionRead)

	if err != nil {
		t.Fatalf("expected denial decision, got error: %v", err)
	}
	if decision.Allowed || !decision.NotFound {
		t.Fatalf("expected hidden personal resource, got %+v", decision)
	}
}

func TestResolveSystemAdminGetsMetadataOnlyForPersonalKB(t *testing.T) {
	resolver := NewResolver(&fakeScopeStore{
		tenants: map[uint64]*types.Tenant{1: {ID: 1, SpaceType: types.SpaceTypePersonal}},
		kbs:     map[string]*types.KnowledgeBase{"kb-1": {ID: "kb-1", TenantID: 1}},
		members: map[string]*types.TenantMember{},
	})

	decision, err := resolver.Resolve(context.Background(), Actor{UserID: "admin", IsSystemAdmin: true}, Resource{Kind: ResourceKnowledgeBase, ID: "kb-1"}, ActionMetadataRead)

	if err != nil {
		t.Fatalf("expected metadata allow, got error: %v", err)
	}
	if !decision.Allowed || !decision.MetadataOnly || decision.NotFound {
		t.Fatalf("expected metadata-only allow, got %+v", decision)
	}
	if len(decision.Scopes) != 1 || len(decision.Scopes[0].AllowedFields) == 0 {
		t.Fatalf("expected metadata allowlist, got %+v", decision.Scopes)
	}
}
