package space

import (
	"context"
	"errors"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

type fakePersonalSpaceStore struct {
	existingTenant *types.Tenant
	createTenant   *types.Tenant
	createMember   *types.TenantMember
	teamTenant     *types.Tenant
}

func (f *fakePersonalSpaceStore) GetPersonalSpace(ctx context.Context, userID string) (*types.Tenant, error) {
	if f.existingTenant == nil {
		return nil, ErrPersonalSpaceNotFound
	}
	return f.existingTenant, nil
}

func (f *fakePersonalSpaceStore) CreatePersonalSpace(ctx context.Context, userID string, tenant *types.Tenant, member *types.TenantMember) (*types.Tenant, error) {
	f.createTenant = tenant
	f.createMember = member
	tenant.ID = 42
	return tenant, nil
}

func (f *fakePersonalSpaceStore) EnsureTeamDefaults(ctx context.Context, userID string, tenant *types.Tenant) error {
	f.teamTenant = tenant
	return nil
}

func TestGetOrCreatePersonalSpaceReturnsExisting(t *testing.T) {
	store := &fakePersonalSpaceStore{
		existingTenant: &types.Tenant{ID: 7, SpaceType: types.SpaceTypePersonal},
	}
	service := NewService(store)

	tenant, err := service.GetOrCreatePersonalSpace(context.Background(), "user-1", "顾测")

	if err != nil {
		t.Fatalf("expected existing personal space, got error: %v", err)
	}
	if tenant.ID != 7 {
		t.Fatalf("expected existing tenant id 7, got %d", tenant.ID)
	}
	if store.createTenant != nil {
		t.Fatal("expected existing personal space to skip create path")
	}
}

func TestGetOrCreatePersonalSpaceCreatesPersonalTenantAndOwner(t *testing.T) {
	store := &fakePersonalSpaceStore{}
	service := NewService(store)

	tenant, err := service.GetOrCreatePersonalSpace(context.Background(), "user-1", "顾测")

	if err != nil {
		t.Fatalf("expected personal space creation, got error: %v", err)
	}
	if tenant.ID != 42 {
		t.Fatalf("expected created tenant id 42, got %d", tenant.ID)
	}
	if store.createTenant == nil {
		t.Fatal("expected tenant creation request")
	}
	if store.createTenant.SpaceType != types.SpaceTypePersonal {
		t.Fatalf("expected personal tenant, got %q", store.createTenant.SpaceType)
	}
	if store.createMember == nil {
		t.Fatal("expected owner membership creation request")
	}
	if store.createMember.UserID != "user-1" || store.createMember.Role != types.TenantRoleOwner {
		t.Fatalf("expected owner membership for user-1, got %+v", store.createMember)
	}
}

func TestGetOrCreatePersonalSpacePropagatesStoreErrors(t *testing.T) {
	wantErr := errors.New("db unavailable")
	store := &failingPersonalSpaceStore{err: wantErr}
	service := NewService(store)

	_, err := service.GetOrCreatePersonalSpace(context.Background(), "user-1", "顾测")

	if !errors.Is(err, wantErr) {
		t.Fatalf("expected store error %v, got %v", wantErr, err)
	}
}

func TestEnsureTeamDefaultsDelegatesToStore(t *testing.T) {
	store := &fakePersonalSpaceStore{}
	service := NewService(store)
	team := &types.Tenant{ID: 80, SpaceType: types.SpaceTypeTeam}

	if err := service.EnsureTeamDefaults(context.Background(), "user-1", team); err != nil {
		t.Fatalf("expected team defaults delegation, got error: %v", err)
	}
	if store.teamTenant != team {
		t.Fatalf("expected team tenant to be passed through, got %+v", store.teamTenant)
	}
}

type failingPersonalSpaceStore struct {
	err error
}

func (f *failingPersonalSpaceStore) GetPersonalSpace(ctx context.Context, userID string) (*types.Tenant, error) {
	return nil, f.err
}

func (f *failingPersonalSpaceStore) CreatePersonalSpace(ctx context.Context, userID string, tenant *types.Tenant, member *types.TenantMember) (*types.Tenant, error) {
	return nil, f.err
}

func (f *failingPersonalSpaceStore) EnsureTeamDefaults(ctx context.Context, userID string, tenant *types.Tenant) error {
	return f.err
}
