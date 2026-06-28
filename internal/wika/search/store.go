package search

import (
	"context"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GormStore 使用现有 GORM 连接读写 Wika search 需要的数据。
type GormStore struct {
	db *gorm.DB
}

// NewGormStore 创建 search store。
func NewGormStore(db *gorm.DB) *GormStore {
	return &GormStore{db: db}
}

// ListReadableScopes 列出用户个人空间和可选团队空间内的文档知识库。
func (s *GormStore) ListReadableScopes(ctx context.Context, userID string, includeTeam bool) ([]ReadableScope, error) {
	scopes := make([]ReadableScope, 0)

	var personal struct {
		TenantID uint64
		KBID     string
	}
	err := s.db.WithContext(ctx).
		Table("user_personal_spaces AS ups").
		Select("ups.tenant_id AS tenant_id, wsd.default_kb_id AS kb_id").
		Joins("JOIN wika_space_defaults AS wsd ON wsd.tenant_id = ups.tenant_id").
		Where("ups.user_id = ?", userID).
		Scan(&personal).Error
	if err != nil {
		return nil, err
	}
	if personal.TenantID != 0 && personal.KBID != "" {
		scopes = append(scopes, ReadableScope{TenantID: personal.TenantID, KBID: personal.KBID, Source: SourcePersonal})
	}

	if !includeTeam {
		return scopes, nil
	}

	var teamRows []struct {
		TenantID uint64
		KBID     string
	}
	err = s.db.WithContext(ctx).
		Table("tenant_members AS tm").
		Select("kb.tenant_id AS tenant_id, kb.id AS kb_id").
		Joins("JOIN tenants AS t ON t.id = tm.tenant_id AND t.space_type = ?", types.SpaceTypeTeam).
		Joins("JOIN knowledge_bases AS kb ON kb.tenant_id = tm.tenant_id AND kb.type = ?", types.KnowledgeBaseTypeDocument).
		Where("tm.user_id = ? AND tm.status = ?", userID, types.TenantMemberStatusActive).
		Scan(&teamRows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range teamRows {
		if row.TenantID == 0 || row.KBID == "" {
			continue
		}
		scopes = append(scopes, ReadableScope{TenantID: row.TenantID, KBID: row.KBID, Source: SourceTeam})
	}
	return scopes, nil
}

// GetKnowledgeStates 批量读取 Wika 知识状态。
func (s *GormStore) GetKnowledgeStates(ctx context.Context, knowledgeIDs []string) (map[string]*types.WikaKnowledgeState, error) {
	out := make(map[string]*types.WikaKnowledgeState, len(knowledgeIDs))
	if len(knowledgeIDs) == 0 {
		return out, nil
	}
	var states []*types.WikaKnowledgeState
	if err := s.db.WithContext(ctx).Where("knowledge_id IN ?", knowledgeIDs).Find(&states).Error; err != nil {
		return nil, err
	}
	for _, state := range states {
		if state != nil {
			out[state.KnowledgeID] = state
		}
	}
	return out, nil
}

// RecordAccess 批量 upsert 搜索访问日聚合；调用方会忽略失败以保护搜索热路径。
func (s *GormStore) RecordAccess(ctx context.Context, records []AccessRecord) error {
	if len(records) == 0 {
		return nil
	}
	items := make([]types.KnowledgeAccessDaily, 0, len(records))
	for _, record := range records {
		accessedAt := record.AccessedAt
		if accessedAt.IsZero() {
			accessedAt = time.Now()
		}
		day := time.Date(accessedAt.Year(), accessedAt.Month(), accessedAt.Day(), 0, 0, 0, 0, accessedAt.Location())
		items = append(items, types.KnowledgeAccessDaily{
			TenantID:       record.TenantID,
			KBID:           record.KBID,
			KnowledgeID:    record.KnowledgeID,
			Day:            day,
			AccessCount:    1,
			LastAccessedAt: accessedAt,
		})
	}
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "tenant_id"}, {Name: "kb_id"}, {Name: "knowledge_id"}, {Name: "day"}},
		DoUpdates: clause.Assignments(map[string]any{
			"access_count":     gorm.Expr("knowledge_access_daily.access_count + EXCLUDED.access_count"),
			"last_accessed_at": gorm.Expr("GREATEST(knowledge_access_daily.last_accessed_at, EXCLUDED.last_accessed_at)"),
		}),
	}).Create(&items).Error
}
