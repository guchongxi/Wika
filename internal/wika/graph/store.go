package graph

import (
	"context"
	"errors"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/gorm"
)

// GormStore 使用 GORM 读取图谱读模型。
type GormStore struct {
	db *gorm.DB
}

func NewGormStore(db *gorm.DB) *GormStore {
	return &GormStore{db: db}
}

func (s *GormStore) Overview(ctx context.Context, tenantID uint64, kbID string) (*Overview, error) {
	var entityCount int64
	if err := s.db.WithContext(ctx).Model(&types.WikaGraphEntity{}).
		Where("tenant_id = ? AND kb_id = ?", tenantID, kbID).
		Count(&entityCount).Error; err != nil {
		return nil, err
	}
	var edgeCount int64
	if err := s.db.WithContext(ctx).Model(&types.WikaGraphEdge{}).
		Where("tenant_id = ? AND kb_id = ?", tenantID, kbID).
		Count(&edgeCount).Error; err != nil {
		return nil, err
	}
	return &Overview{TenantID: tenantID, KBID: kbID, EntityCount: entityCount, EdgeCount: edgeCount}, nil
}

func (s *GormStore) OverviewByKB(ctx context.Context, kbID string) (*Overview, error) {
	var kb types.KnowledgeBase
	if err := s.db.WithContext(ctx).
		Select("id", "tenant_id").
		First(&kb, "id = ?", kbID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &Overview{KBID: kbID}, nil
		}
		return nil, err
	}
	var entityCount int64
	if err := s.db.WithContext(ctx).Model(&types.WikaGraphEntity{}).
		Where("kb_id = ?", kbID).
		Count(&entityCount).Error; err != nil {
		return nil, err
	}
	var edgeCount int64
	if err := s.db.WithContext(ctx).Model(&types.WikaGraphEdge{}).
		Where("kb_id = ?", kbID).
		Count(&edgeCount).Error; err != nil {
		return nil, err
	}
	return &Overview{TenantID: kb.TenantID, KBID: kbID, EntityCount: entityCount, EdgeCount: edgeCount}, nil
}

func (s *GormStore) ListEntities(ctx context.Context, input ListEntitiesInput) ([]*types.WikaGraphEntity, int64, error) {
	query := s.db.WithContext(ctx).Model(&types.WikaGraphEntity{}).
		Where("tenant_id = ? AND kb_id = ?", input.TenantID, input.KBID)
	if input.EntityType != "" {
		query = query.Where("entity_type = ?", input.EntityType)
	}
	if strings.TrimSpace(input.Query) != "" {
		like := "%" + strings.TrimSpace(input.Query) + "%"
		query = query.Where("name LIKE ?", like)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var entities []*types.WikaGraphEntity
	err := query.
		Order("entity_type ASC, name ASC, id ASC").
		Limit(normalizeLimit(input.Limit)).
		Offset(normalizeOffset(input.Offset)).
		Find(&entities).Error
	return entities, total, err
}

func (s *GormStore) GetEntity(ctx context.Context, tenantID uint64, kbID string, entityID uint64) (*types.WikaGraphEntity, error) {
	var entity types.WikaGraphEntity
	err := s.db.WithContext(ctx).First(&entity, "id = ? AND tenant_id = ? AND kb_id = ?", entityID, tenantID, kbID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrGraphEntityNotFound
	}
	if err != nil {
		return nil, err
	}
	return &entity, nil
}

func (s *GormStore) ListEdges(ctx context.Context, input ListEdgesInput) ([]*types.WikaGraphEdge, int64, error) {
	query := s.db.WithContext(ctx).Model(&types.WikaGraphEdge{}).
		Where("tenant_id = ? AND kb_id = ?", input.TenantID, input.KBID)
	if input.SourceEntityID != 0 {
		query = query.Where("source_entity_id = ?", input.SourceEntityID)
	}
	if input.TargetEntityID != 0 {
		query = query.Where("target_entity_id = ?", input.TargetEntityID)
	}
	if input.EvidenceKnowledge != "" {
		query = query.Where("evidence_knowledge_id = ?", input.EvidenceKnowledge)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var edges []*types.WikaGraphEdge
	err := query.
		Order("relation_type ASC, id ASC").
		Limit(normalizeLimit(input.Limit)).
		Offset(normalizeOffset(input.Offset)).
		Find(&edges).Error
	return edges, total, err
}

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return 50
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func normalizeOffset(offset int) int {
	if offset < 0 {
		return 0
	}
	return offset
}
