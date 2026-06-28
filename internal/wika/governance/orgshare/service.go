package orgshare

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

type TeamAdminChecker interface {
	CanAdminTenant(ctx context.Context, actorID string, tenantID uint64) bool
}

type FeatureGate interface {
	GetBool(ctx context.Context, key string, envName string, def bool) bool
}

type AuditLogger interface {
	Log(ctx context.Context, entry *types.AuditLog) error
}

type Service struct {
	store Store
	admin TeamAdminChecker
	flags FeatureGate
	audit AuditLogger
}

type ServiceOption func(*Service)

const FeatureFlagKey = "wika.governance.org_share.enabled"

func WithFeatureGate(flags FeatureGate) ServiceOption {
	return func(s *Service) {
		s.flags = flags
	}
}

func WithAuditLogger(audit AuditLogger) ServiceOption {
	return func(s *Service) {
		s.audit = audit
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
	var share *types.WikaOrgShare
	err = s.withTransaction(ctx, func(txCtx context.Context, store Store) error {
		created, err := store.CreateShare(ctx, &types.WikaOrgShare{
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
		if err != nil {
			return err
		}
		if err := s.logShareAudit(txCtx, types.AuditActionWikaOrgShareCreated, input.ActorID, "", created.Status, created, created.SourceTenantID); err != nil {
			return err
		}
		share = created
		return nil
	})
	if err != nil {
		return nil, err
	}
	return share, nil
}

func (s *Service) AcceptShare(ctx context.Context, input AcceptShareInput) (*types.WikaOrgShare, error) {
	if !s.featureEnabled(ctx) {
		return nil, ErrFeatureDisabled
	}
	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}
	var updated *types.WikaOrgShare
	err := s.withTransaction(ctx, func(txCtx context.Context, store Store) error {
		share, err := store.GetShare(ctx, input.ShareID)
		if err != nil {
			return err
		}
		if share.Status != types.WikaOrgShareStatusPending {
			return ErrInvalidShareState
		}
		if !s.canAdmin(ctx, input.ActorID, share.TargetTenantID) {
			return ErrScopeDenied
		}
		next, err := store.UpdateShare(ctx, input.ShareID, map[string]any{
			"status":      types.WikaOrgShareStatusActive,
			"accepted_by": input.ActorID,
			"accepted_at": now,
			"updated_at":  now,
		})
		if err != nil {
			return err
		}
		if err := s.logShareAudit(txCtx, types.AuditActionWikaOrgShareAccepted, input.ActorID, share.Status, next.Status, next, next.TargetTenantID); err != nil {
			return err
		}
		updated = next
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func (s *Service) RevokeShare(ctx context.Context, input RevokeShareInput) (*types.WikaOrgShare, error) {
	if !s.featureEnabled(ctx) {
		return nil, ErrFeatureDisabled
	}
	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}
	var updated *types.WikaOrgShare
	err := s.withTransaction(ctx, func(txCtx context.Context, store Store) error {
		share, err := store.GetShare(ctx, input.ShareID)
		if err != nil {
			return err
		}
		if share.Status == types.WikaOrgShareStatusRevoked {
			return ErrInvalidShareState
		}
		if !s.canAdmin(ctx, input.ActorID, share.SourceTenantID) && !s.canAdmin(ctx, input.ActorID, share.TargetTenantID) {
			return ErrScopeDenied
		}
		next, err := store.UpdateShare(ctx, input.ShareID, map[string]any{
			"status":     types.WikaOrgShareStatusRevoked,
			"revoked_by": input.ActorID,
			"revoked_at": now,
			"updated_at": now,
		})
		if err != nil {
			return err
		}
		if err := s.logShareAudit(txCtx, types.AuditActionWikaOrgShareRevoked, input.ActorID, share.Status, next.Status, next, s.actorTenantForRevoke(ctx, input.ActorID, share)); err != nil {
			return err
		}
		updated = next
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func (s *Service) withTransaction(ctx context.Context, fn func(context.Context, Store) error) error {
	if txStore, ok := s.store.(transactionalStore); ok {
		return txStore.WithTransaction(ctx, fn)
	}
	return fn(ctx, s.store)
}

func (s *Service) featureEnabled(ctx context.Context) bool {
	if s.flags == nil {
		return false
	}
	return s.flags.GetBool(ctx, FeatureFlagKey, "", false)
}

func (s *Service) canAdmin(ctx context.Context, actorID string, tenantID uint64) bool {
	return s.admin != nil && s.admin.CanAdminTenant(ctx, actorID, tenantID)
}

func normalizeAllowedFields(fields []string) ([]string, error) {
	if len(fields) == 0 {
		return DefaultAllowedFields(), nil
	}
	for _, field := range fields {
		if !IsAllowedField(field) {
			return nil, ErrInvalidAllowedFields
		}
	}
	return append([]string(nil), fields...), nil
}

func DefaultAllowedFields() []string {
	return []string{"id", "title", "source_tenant_id", "source_kb_id", "quality_score", "freshness_status"}
}

func IsAllowedField(field string) bool {
	switch field {
	case "id", "title", "source_tenant_id", "source_kb_id", "quality_score", "freshness_status":
		return true
	default:
		return false
	}
}

func SanitizeAllowedFields(fields []string) []string {
	if len(fields) == 0 {
		return DefaultAllowedFields()
	}
	out := make([]string, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		if !IsAllowedField(field) {
			continue
		}
		if _, ok := seen[field]; ok {
			continue
		}
		seen[field] = struct{}{}
		out = append(out, field)
	}
	return out
}

func (s *Service) logShareAudit(ctx context.Context, action types.AuditAction, actorID string, oldStatus string, newStatus string, share *types.WikaOrgShare, actorTenantID uint64) error {
	if s.audit == nil || share == nil {
		return nil
	}
	if actorTenantID == 0 {
		actorTenantID = share.SourceTenantID
	}
	details, _ := json.Marshal(map[string]any{
		"share_id":         share.ID,
		"org_id":           share.OrgID,
		"actor_tenant_id":  actorTenantID,
		"source_tenant_id": share.SourceTenantID,
		"source_kb_id":     share.SourceKBID,
		"target_tenant_id": share.TargetTenantID,
		"old_status":       oldStatus,
		"new_status":       newStatus,
		"allowed_fields":   SanitizeAllowedFields(decodeShareAllowedFields(share.AllowedFields)),
	})
	return s.audit.Log(ctx, &types.AuditLog{
		TenantID:    actorTenantID,
		ActorUserID: actorID,
		Action:      action,
		TargetType:  "wika_org_share",
		TargetID:    strconv.FormatUint(share.ID, 10),
		Details:     types.JSON(details),
	})
}

func (s *Service) actorTenantForRevoke(ctx context.Context, actorID string, share *types.WikaOrgShare) uint64 {
	if share == nil {
		return 0
	}
	if s.canAdmin(ctx, actorID, share.SourceTenantID) {
		return share.SourceTenantID
	}
	if s.canAdmin(ctx, actorID, share.TargetTenantID) {
		return share.TargetTenantID
	}
	return 0
}

func decodeShareAllowedFields(raw types.JSON) []string {
	if len(raw) == 0 {
		return nil
	}
	var fields []string
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil
	}
	return fields
}
