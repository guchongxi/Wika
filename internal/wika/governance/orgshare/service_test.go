package orgshare

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
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

type fakeOrgShareAudit struct {
	entries []*types.AuditLog
	err     error
}

func (a *fakeOrgShareAudit) Log(ctx context.Context, entry *types.AuditLog) error {
	if a.err != nil {
		return a.err
	}
	a.entries = append(a.entries, entry)
	return nil
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

func TestServiceWritesAuditForShareLifecycle(t *testing.T) {
	db := setupOrgShareTestDB(t)
	audit := &fakeOrgShareAudit{}
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	svc := NewService(
		NewGormStore(db),
		fakeTeamAdminChecker{adminTenants: map[uint64]bool{80: true}},
		WithFeatureGate(fakeOrgShareGate{enabled: true}),
		WithAuditLogger(audit),
	)

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

	svc = NewService(
		NewGormStore(db),
		fakeTeamAdminChecker{adminTenants: map[uint64]bool{80: true, 90: true}},
		WithFeatureGate(fakeOrgShareGate{enabled: true}),
		WithAuditLogger(audit),
	)
	_, err = svc.AcceptShare(context.Background(), AcceptShareInput{ActorID: "u-target", ShareID: share.ID, Now: now.Add(time.Minute)})
	require.NoError(t, err)
	_, err = svc.RevokeShare(context.Background(), RevokeShareInput{ActorID: "u-source", ShareID: share.ID, Now: now.Add(2 * time.Minute)})
	require.NoError(t, err)

	if len(audit.entries) != 3 {
		t.Fatalf("expected create/accept/revoke audit entries, got %+v", audit.entries)
	}
	assertOrgShareAuditEntry(t, audit.entries[0], types.AuditActionWikaOrgShareCreated, "u-source", share.ID)
	assertOrgShareAuditEntry(t, audit.entries[1], types.AuditActionWikaOrgShareAccepted, "u-target", share.ID)
	assertOrgShareAuditEntry(t, audit.entries[2], types.AuditActionWikaOrgShareRevoked, "u-source", share.ID)
}

func assertOrgShareAuditEntry(t *testing.T, entry *types.AuditLog, action types.AuditAction, actorID string, shareID uint64) {
	t.Helper()
	if entry == nil ||
		entry.Action != action ||
		entry.ActorUserID != actorID ||
		entry.TargetType != "wika_org_share" ||
		entry.TargetID != strconv.FormatUint(shareID, 10) {
		t.Fatalf("unexpected org share audit entry: %+v", entry)
	}
}

func TestServiceAuditDetailsRecordShareStateTransitionWithoutSensitiveFields(t *testing.T) {
	db := setupOrgShareTestDB(t)
	audit := &fakeOrgShareAudit{}
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	svc := NewService(
		NewGormStore(db),
		fakeTeamAdminChecker{adminTenants: map[uint64]bool{80: true, 90: true}},
		WithFeatureGate(fakeOrgShareGate{enabled: true}),
		WithAuditLogger(audit),
	)

	share, err := svc.CreateShare(context.Background(), CreateShareInput{
		ActorID:        "u-owner",
		OrgID:          "org-1",
		SourceTenantID: 80,
		SourceKBID:     "kb-source",
		TargetTenantID: 90,
		AllowedFields:  []string{"id", "title"},
		Now:            now,
	})
	require.NoError(t, err)
	_, err = svc.RevokeShare(context.Background(), RevokeShareInput{ActorID: "u-owner", ShareID: share.ID, Now: now.Add(time.Minute)})
	require.NoError(t, err)

	if len(audit.entries) != 2 {
		t.Fatalf("expected create/revoke audit entries, got %+v", audit.entries)
	}
	assertOrgShareAuditDetails(t, audit.entries[0], map[string]any{
		"old_status":       "",
		"new_status":       string(types.WikaOrgShareStatusActive),
		"actor_tenant_id":  float64(80),
		"source_tenant_id": float64(80),
		"target_tenant_id": float64(90),
	})
	assertOrgShareAuditDetails(t, audit.entries[1], map[string]any{
		"old_status":       string(types.WikaOrgShareStatusActive),
		"new_status":       string(types.WikaOrgShareStatusRevoked),
		"actor_tenant_id":  float64(80),
		"source_tenant_id": float64(80),
		"target_tenant_id": float64(90),
	})
}

func assertOrgShareAuditDetails(t *testing.T, entry *types.AuditLog, expected map[string]any) {
	t.Helper()
	var details map[string]any
	require.NoError(t, json.Unmarshal(entry.Details, &details))
	for key, value := range expected {
		if details[key] != value {
			t.Fatalf("expected audit details[%s]=%v, got %v in %+v", key, value, details[key], details)
		}
	}
	for _, forbidden := range []string{"content", "chunk", "snippet", "evidence_text", "file", "file_path", "token_hash"} {
		if _, ok := details[forbidden]; ok {
			t.Fatalf("audit details must not contain %s: %+v", forbidden, details)
		}
	}
}

func TestServiceDoesNotAdvanceShareStateWhenAuditFails(t *testing.T) {
	db := setupOrgShareTestDB(t)
	auditErr := errors.New("audit unavailable")
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	svc := NewService(
		NewGormStore(db),
		fakeTeamAdminChecker{adminTenants: map[uint64]bool{80: true}},
		WithFeatureGate(fakeOrgShareGate{enabled: true}),
		WithAuditLogger(&fakeOrgShareAudit{err: auditErr}),
	)

	_, err := svc.CreateShare(context.Background(), CreateShareInput{
		ActorID:        "u-source",
		OrgID:          "org-1",
		SourceTenantID: 80,
		SourceKBID:     "kb-source",
		TargetTenantID: 90,
		AllowedFields:  []string{"id", "title"},
		Now:            now,
	})
	if !errors.Is(err, auditErr) {
		t.Fatalf("expected audit error from create, got %v", err)
	}
	var count int64
	require.NoError(t, db.Model(&types.WikaOrgShare{}).Count(&count).Error)
	if count != 0 {
		t.Fatalf("audit-failed create must not persist share, got count=%d", count)
	}

	successAudit := &fakeOrgShareAudit{}
	svc = NewService(
		NewGormStore(db),
		fakeTeamAdminChecker{adminTenants: map[uint64]bool{80: true}},
		WithFeatureGate(fakeOrgShareGate{enabled: true}),
		WithAuditLogger(successAudit),
	)
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

	svc = NewService(
		NewGormStore(db),
		fakeTeamAdminChecker{adminTenants: map[uint64]bool{80: true, 90: true}},
		WithFeatureGate(fakeOrgShareGate{enabled: true}),
		WithAuditLogger(&fakeOrgShareAudit{err: auditErr}),
	)
	_, err = svc.AcceptShare(context.Background(), AcceptShareInput{ActorID: "u-target", ShareID: share.ID, Now: now.Add(time.Minute)})
	if !errors.Is(err, auditErr) {
		t.Fatalf("expected audit error from accept, got %v", err)
	}
	var persisted types.WikaOrgShare
	require.NoError(t, db.First(&persisted, "id = ?", share.ID).Error)
	if persisted.Status != types.WikaOrgShareStatusPending || persisted.AcceptedBy != "" || persisted.AcceptedAt != nil {
		t.Fatalf("audit-failed accept must keep pending state, got %+v", persisted)
	}

	svc = NewService(
		NewGormStore(db),
		fakeTeamAdminChecker{adminTenants: map[uint64]bool{80: true, 90: true}},
		WithFeatureGate(fakeOrgShareGate{enabled: true}),
		WithAuditLogger(successAudit),
	)
	_, err = svc.AcceptShare(context.Background(), AcceptShareInput{ActorID: "u-target", ShareID: share.ID, Now: now.Add(2 * time.Minute)})
	require.NoError(t, err)

	svc = NewService(
		NewGormStore(db),
		fakeTeamAdminChecker{adminTenants: map[uint64]bool{80: true, 90: true}},
		WithFeatureGate(fakeOrgShareGate{enabled: true}),
		WithAuditLogger(&fakeOrgShareAudit{err: auditErr}),
	)
	_, err = svc.RevokeShare(context.Background(), RevokeShareInput{ActorID: "u-source", ShareID: share.ID, Now: now.Add(3 * time.Minute)})
	if !errors.Is(err, auditErr) {
		t.Fatalf("expected audit error from revoke, got %v", err)
	}
	require.NoError(t, db.First(&persisted, "id = ?", share.ID).Error)
	if persisted.Status != types.WikaOrgShareStatusActive || persisted.RevokedBy != "" || persisted.RevokedAt != nil {
		t.Fatalf("audit-failed revoke must keep active state, got %+v", persisted)
	}
}
