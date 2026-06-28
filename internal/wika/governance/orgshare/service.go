package orgshare

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

type TeamAdminChecker interface {
	CanAdminTenant(ctx context.Context, actorID string, tenantID uint64) bool
}

type FeatureGate interface {
	GetBool(ctx context.Context, key string, envName string, def bool) bool
}

type Service struct {
	store Store
	admin TeamAdminChecker
	flags FeatureGate
}

type ServiceOption func(*Service)

const orgShareFeatureFlagKey = "wika.governance.org_share.enabled"

func WithFeatureGate(flags FeatureGate) ServiceOption {
	return func(s *Service) {
		s.flags = flags
	}
}

func NewService(store Store, admin TeamAdminChecker, opts ...ServiceOption) *Service {
	svc := &Service{store: store, admin: admin}
	for _, opt := range opts {
		opt(svc)
	}
	return svc
}

func (s *Service) CreateShare(ctx context.Context, input CreateShareInput) (*types.WikaOrgShare, error) {
	if !s.featureEnabled(ctx) {
		return nil, ErrFeatureDisabled
	}
	if !s.canAdmin(ctx, input.ActorID, input.SourceTenantID) {
		return nil, ErrScopeDenied
	}
	fields, err := normalizeAllowedFields(input.AllowedFields)
	if err != nil {
		return nil, err
	}
	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}
	status := types.WikaOrgShareStatusPending
	acceptedBy := ""
	var acceptedAt *time.Time
	if s.canAdmin(ctx, input.ActorID, input.TargetTenantID) {
		status = types.WikaOrgShareStatusActive
		acceptedBy = input.ActorID
		acceptedAt = &now
	}
	rawFields, _ := json.Marshal(fields)
	return s.store.CreateShare(ctx, &types.WikaOrgShare{
		OrgID:          input.OrgID,
		SourceTenantID: input.SourceTenantID,
		SourceKBID:     input.SourceKBID,
		TargetTenantID: input.TargetTenantID,
		Mode:           types.WikaOrgShareModeReference,
		AllowedFields:  types.JSON(rawFields),
		Status:         status,
		CreatedBy:      input.ActorID,
		AcceptedBy:     acceptedBy,
		AcceptedAt:     acceptedAt,
		CreatedAt:      now,
		UpdatedAt:      now,
	})
}

func (s *Service) AcceptShare(ctx context.Context, input AcceptShareInput) (*types.WikaOrgShare, error) {
	if !s.featureEnabled(ctx) {
		return nil, ErrFeatureDisabled
	}
	share, err := s.store.GetShare(ctx, input.ShareID)
	if err != nil {
		return nil, err
	}
	if share.Status != types.WikaOrgShareStatusPending {
		return nil, ErrInvalidShareState
	}
	if !s.canAdmin(ctx, input.ActorID, share.TargetTenantID) {
		return nil, ErrScopeDenied
	}
	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}
	return s.store.UpdateShare(ctx, input.ShareID, map[string]any{
		"status":      types.WikaOrgShareStatusActive,
		"accepted_by": input.ActorID,
		"accepted_at": now,
		"updated_at":  now,
	})
}

func (s *Service) RevokeShare(ctx context.Context, input RevokeShareInput) (*types.WikaOrgShare, error) {
	if !s.featureEnabled(ctx) {
		return nil, ErrFeatureDisabled
	}
	share, err := s.store.GetShare(ctx, input.ShareID)
	if err != nil {
		return nil, err
	}
	if share.Status == types.WikaOrgShareStatusRevoked {
		return nil, ErrInvalidShareState
	}
	if !s.canAdmin(ctx, input.ActorID, share.SourceTenantID) && !s.canAdmin(ctx, input.ActorID, share.TargetTenantID) {
		return nil, ErrScopeDenied
	}
	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}
	return s.store.UpdateShare(ctx, input.ShareID, map[string]any{
		"status":     types.WikaOrgShareStatusRevoked,
		"revoked_by": input.ActorID,
		"revoked_at": now,
		"updated_at": now,
	})
}

func (s *Service) featureEnabled(ctx context.Context) bool {
	if s.flags == nil {
		return false
	}
	return s.flags.GetBool(ctx, orgShareFeatureFlagKey, "", false)
}

func (s *Service) canAdmin(ctx context.Context, actorID string, tenantID uint64) bool {
	return s.admin != nil && s.admin.CanAdminTenant(ctx, actorID, tenantID)
}

func normalizeAllowedFields(fields []string) ([]string, error) {
	if len(fields) == 0 {
		return []string{"id", "title", "source_tenant_id", "source_kb_id", "quality_score", "freshness_status"}, nil
	}
	allowed := map[string]bool{
		"id":               true,
		"title":            true,
		"source_tenant_id": true,
		"source_kb_id":     true,
		"quality_score":    true,
		"freshness_status": true,
	}
	for _, field := range fields {
		if !allowed[field] {
			return nil, ErrInvalidAllowedFields
		}
	}
	return fields, nil
}
