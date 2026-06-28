package scope

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/Tencent/WeKnora/internal/types"
	wikaorgshare "github.com/Tencent/WeKnora/internal/wika/governance/orgshare"
	"gorm.io/gorm"
)

type GormStore struct {
	db *gorm.DB
}

func NewGormStore(db *gorm.DB) *GormStore {
	return &GormStore{db: db}
}

func (s *GormStore) GetKnowledgeBase(ctx context.Context, kbID string) (*types.KnowledgeBase, error) {
	var kb types.KnowledgeBase
	err := s.db.WithContext(ctx).Where("id = ?", kbID).First(&kb).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrResourceNotFound
		}
		return nil, err
	}
	return &kb, nil
}

func (s *GormStore) GetTenant(ctx context.Context, tenantID uint64) (*types.Tenant, error) {
	var tenant types.Tenant
	err := s.db.WithContext(ctx).Where("id = ?", tenantID).First(&tenant).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrResourceNotFound
		}
		return nil, err
	}
	return &tenant, nil
}

func (s *GormStore) GetTenantMember(ctx context.Context, userID string, tenantID uint64) (*types.TenantMember, error) {
	var member types.TenantMember
	err := s.db.WithContext(ctx).
		Where("user_id = ? AND tenant_id = ? AND status = ?", userID, tenantID, types.TenantMemberStatusActive).
		First(&member).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrResourceNotFound
		}
		return nil, err
	}
	return &member, nil
}

func (s *GormStore) ListSharedKnowledgeBaseScopes(ctx context.Context, userID string, kbID string) ([]Scope, error) {
	var rows []struct {
		TenantID      uint64
		KBID          string
		AllowedFields types.JSON
	}
	err := s.db.WithContext(ctx).
		Table("wika_org_shares AS ws").
		Select("ws.source_tenant_id AS tenant_id, ws.source_kb_id AS kb_id, ws.allowed_fields AS allowed_fields").
		Joins("JOIN tenant_members AS tm ON tm.tenant_id = ws.target_tenant_id AND tm.user_id = ? AND tm.status = ?", userID, types.TenantMemberStatusActive).
		Joins("JOIN tenants AS source_tenant ON source_tenant.id = ws.source_tenant_id AND source_tenant.space_type = ?", types.SpaceTypeTeam).
		Joins("JOIN knowledge_bases AS kb ON kb.id = ws.source_kb_id AND kb.tenant_id = ws.source_tenant_id AND kb.type = ?", types.KnowledgeBaseTypeDocument).
		Where("ws.source_kb_id = ? AND ws.status = ?", kbID, types.WikaOrgShareStatusActive).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	scopes := make([]Scope, 0, len(rows))
	for _, row := range rows {
		if row.TenantID == 0 || row.KBID == "" {
			continue
		}
		scopes = append(scopes, Scope{
			TenantID:      row.TenantID,
			KBID:          row.KBID,
			Source:        ScopeSourceShared,
			AllowedFields: decodeAllowedFields(row.AllowedFields),
		})
	}
	return scopes, nil
}

func decodeAllowedFields(raw types.JSON) []string {
	if len(raw) == 0 {
		return nil
	}
	var fields []string
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil
	}
	return wikaorgshare.SanitizeAllowedFields(fields)
}
