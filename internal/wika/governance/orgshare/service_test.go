package orgshare

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type fakeTeamAdminChecker struct {
	adminTenants map[uint64]bool
}

func (c fakeTeamAdminChecker) CanAdminTenant(_ context.Context, _ string, tenantID uint64) bool {
	return c.adminTenants[tenantID]
}

type fakeOrgShareGate struct {
	enabled bool
}

func (g fakeOrgShareGate) GetBool(ctx context.Context, key string, envName string, def bool) bool {
	return g.enabled
}

func setupOrgShareTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&types.WikaOrgShare{}))
	return db
}

func TestServiceCreateShareRejectsUnsafeAllowedFields(t *testing.T) {
	svc := NewService(NewGormStore(setupOrgShareTestDB(t)), fakeTeamAdminChecker{adminTenants: map[uint64]bool{80: true}}, WithFeatureGate(fakeOrgShareGate{enabled: true}))

	_, err := svc.CreateShare(context.Background(), CreateShareInput{
		ActorID:        "u-owner",
		OrgID:          "org-1",
		SourceTenantID: 80,
		SourceKBID:     "kb-source",
		TargetTenantID: 90,
		AllowedFields:  []string{"id", "title", "content"},
		Now:            time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC),
	})
	if err != ErrInvalidAllowedFields {
		t.Fatalf("expected ErrInvalidAllowedFields, got %v", err)
	}
}

func TestServiceCreateSharePendingWhenTargetAdminMissing(t *testing.T) {
	svc := NewService(NewGormStore(setupOrgShareTestDB(t)), fakeTeamAdminChecker{adminTenants: map[uint64]bool{80: true}}, WithFeatureGate(fakeOrgShareGate{enabled: true}))

	share, err := svc.CreateShare(context.Background(), CreateShareInput{
		ActorID:        "u-owner",
		OrgID:          "org-1",
		SourceTenantID: 80,
		SourceKBID:     "kb-source",
		TargetTenantID: 90,
		AllowedFields:  []string{"id", "title"},
		Now:            time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	if share.Status != types.WikaOrgShareStatusPending || share.Mode != types.WikaOrgShareModeReference {
		t.Fatalf("expected pending reference share, got %+v", share)
	}
}

func TestServiceCreateShareActiveWhenActorAdminsBothTeams(t *testing.T) {
	svc := NewService(NewGormStore(setupOrgShareTestDB(t)), fakeTeamAdminChecker{adminTenants: map[uint64]bool{80: true, 90: true}}, WithFeatureGate(fakeOrgShareGate{enabled: true}))

	share, err := svc.CreateShare(context.Background(), CreateShareInput{
		ActorID:        "u-owner",
		OrgID:          "org-1",
		SourceTenantID: 80,
		SourceKBID:     "kb-source",
		TargetTenantID: 90,
		AllowedFields:  []string{"id", "title"},
		Now:            time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	if share.Status != types.WikaOrgShareStatusActive || share.AcceptedBy != "u-owner" || share.AcceptedAt == nil {
		t.Fatalf("expected active share accepted by actor, got %+v", share)
	}
}

func TestServiceAcceptAndRevokeShareRequireTeamAdmin(t *testing.T) {
	db := setupOrgShareTestDB(t)
	svc := NewService(NewGormStore(db), fakeTeamAdminChecker{adminTenants: map[uint64]bool{80: true}}, WithFeatureGate(fakeOrgShareGate{enabled: true}))
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	share, err := svc.CreateShare(context.Background(), CreateShareInput{
		ActorID:        "u-source",
		OrgID:          "org-1",
		SourceTenantID: 80,
		SourceKBID:     "kb-source",
		TargetTenantID: 90,
		AllowedFields:  []string{"id", "title"},
		Now:            now,
	})
	require.NoError(t, err)

	_, err = svc.AcceptShare(context.Background(), AcceptShareInput{ActorID: "u-target", ShareID: share.ID, Now: now.Add(time.Minute)})
	if err != ErrScopeDenied {
		t.Fatalf("expected target admin requirement, got %v", err)
	}

	svc = NewService(NewGormStore(db), fakeTeamAdminChecker{adminTenants: map[uint64]bool{80: true, 90: true}}, WithFeatureGate(fakeOrgShareGate{enabled: true}))
	accepted, err := svc.AcceptShare(context.Background(), AcceptShareInput{ActorID: "u-target", ShareID: share.ID, Now: now.Add(time.Minute)})
	require.NoError(t, err)
	if accepted.Status != types.WikaOrgShareStatusActive || accepted.AcceptedBy != "u-target" {
		t.Fatalf("expected accepted share, got %+v", accepted)
	}

	revoked, err := svc.RevokeShare(context.Background(), RevokeShareInput{ActorID: "u-source", ShareID: share.ID, Now: now.Add(2 * time.Minute)})
	require.NoError(t, err)
	if revoked.Status != types.WikaOrgShareStatusRevoked || revoked.RevokedBy != "u-source" {
		t.Fatalf("expected revoked share, got %+v", revoked)
	}
}
