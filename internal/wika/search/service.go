package search

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	wikaorgshare "github.com/Tencent/WeKnora/internal/wika/governance/orgshare"
	wikagraph "github.com/Tencent/WeKnora/internal/wika/graph"
)

// Store 隔离 search 所需的 scope、状态和访问聚合写入。
type Store interface {
	ListReadableScopes(ctx context.Context, userID string, includeTeam bool) ([]ReadableScope, error)
	GetKnowledgeStates(ctx context.Context, knowledgeIDs []string) (map[string]*types.WikaKnowledgeState, error)
	RecordAccess(ctx context.Context, records []AccessRecord) error
}

// KnowledgeSearcher 是复用现有知识搜索能力的最小接口。
type KnowledgeSearcher interface {
	SearchKnowledgeForScopes(ctx context.Context, scopes []types.KnowledgeSearchScope, keyword string, offset, limit int, fileTypes []string) ([]*types.Knowledge, bool, error)
	ListPagedKnowledgeByKnowledgeBaseID(ctx context.Context, kbID string, page *types.Pagination, filter types.KnowledgeListFilter) (*types.PageResult, error)
	GetKnowledgeByIDOnly(ctx context.Context, id string) (*types.Knowledge, error)
}

// GraphSearcher 是 P4 图谱增强检索的 best-effort 适配接口。
type GraphSearcher interface {
	SearchGraph(ctx context.Context, scopes []ReadableScope, query string, limit int) ([]GraphContribution, error)
}

// Service 编排权限感知的 Wika compact 搜索。
type Service struct {
	store     Store
	knowledge KnowledgeSearcher
	graph     GraphSearcher
}

// NewService 创建 Wika search 服务。
func NewService(store *GormStore, knowledge interfaces.KnowledgeService, graphService *wikagraph.Service) *Service {
	return &Service{store: store, knowledge: knowledge, graph: NewGraphSearchAdapter(graphService)}
}

// SearchKnowledge 执行 scope-aware 知识检索并返回 compact 结果。
func (s *Service) SearchKnowledge(ctx context.Context, input SearchInput) (*SearchResult, error) {
	limit := input.Limit
	if limit <= 0 {
		limit = 5
	}
	if limit > 20 {
		limit = 20
	}
	query := strings.TrimSpace(input.Query)
	if query == "" || s.store == nil || s.knowledge == nil {
		return &SearchResult{Results: []ResultItem{}}, nil
	}

	readableScopes, err := s.store.ListReadableScopes(ctx, input.UserID, input.IncludeTeam)
	if err != nil {
		return nil, err
	}
	if len(readableScopes) == 0 {
		return &SearchResult{Results: []ResultItem{}}, nil
	}
	if kbID := strings.TrimSpace(input.KBID); kbID != "" {
		readableScopes = filterReadableScopesByKB(readableScopes, kbID)
		if len(readableScopes) == 0 {
			return &SearchResult{Results: []ResultItem{}}, nil
		}
	}

	graphContribution, graphDegraded := s.searchGraphBestEffort(ctx, readableScopes, query, limit)

	searchScopes := make([]types.KnowledgeSearchScope, 0, len(readableScopes))
	scopeByKey := make(map[string]ReadableScope, len(readableScopes))
	for _, scope := range readableScopes {
		searchScopes = append(searchScopes, types.KnowledgeSearchScope{TenantID: scope.TenantID, KBID: scope.KBID})
		scopeByKey[scopeKey(scope.TenantID, scope.KBID)] = scope
	}

	knowledges, hasMore, err := s.knowledge.SearchKnowledgeForScopes(ctx, searchScopes, query, 0, limit, nil)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(knowledges))
	for _, knowledge := range knowledges {
		if knowledge != nil {
			ids = append(ids, knowledge.ID)
		}
	}
	states, err := s.store.GetKnowledgeStates(ctx, ids)
	if err != nil {
		return nil, err
	}

	results := make([]ResultItem, 0, len(knowledges))
	accessRecords := make([]AccessRecord, 0, len(knowledges))
	now := time.Now()
	for _, knowledge := range knowledges {
		if knowledge == nil {
			continue
		}
		results = append(results, buildResultItem(knowledge, scopeByKey[scopeKey(knowledge.TenantID, knowledge.KnowledgeBaseID)], states[knowledge.ID]))
		accessRecords = append(accessRecords, AccessRecord{
			TenantID:    knowledge.TenantID,
			KBID:        knowledge.KnowledgeBaseID,
			KnowledgeID: knowledge.ID,
			AccessedAt:  now,
		})
	}
	if len(accessRecords) > 0 {
		_ = s.store.RecordAccess(ctx, accessRecords)
	}
	return &SearchResult{Results: results, Truncated: hasMore, GraphContribution: graphContribution, GraphDegraded: graphDegraded}, nil
}

// ListMyKnowledge 列出当前用户个人默认知识库里的知识。
func (s *Service) ListMyKnowledge(ctx context.Context, input MineInput) (*MineResult, error) {
	limit := input.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if s.store == nil || s.knowledge == nil {
		return &MineResult{Results: []ResultItem{}}, nil
	}
	scopes, err := s.store.ListReadableScopes(ctx, input.UserID, false)
	if err != nil {
		return nil, err
	}
	var personal *ReadableScope
	for i := range scopes {
		if scopes[i].Source == SourcePersonal {
			personal = &scopes[i]
			break
		}
	}
	if personal == nil {
		return &MineResult{Results: []ResultItem{}}, nil
	}

	page := &types.Pagination{Page: 1, PageSize: limit}
	filter := types.KnowledgeListFilter{ParseStatus: strings.TrimSpace(input.Status)}
	personalCtx := context.WithValue(ctx, types.TenantIDContextKey, personal.TenantID)
	pageResult, err := s.knowledge.ListPagedKnowledgeByKnowledgeBaseID(personalCtx, personal.KBID, page, filter)
	if err != nil {
		return nil, err
	}
	knowledges := pageResultKnowledges(pageResult)
	ids := make([]string, 0, len(knowledges))
	for _, knowledge := range knowledges {
		if knowledge != nil {
			ids = append(ids, knowledge.ID)
		}
	}
	states, err := s.store.GetKnowledgeStates(ctx, ids)
	if err != nil {
		return nil, err
	}
	results := make([]ResultItem, 0, len(knowledges))
	for _, knowledge := range knowledges {
		if knowledge != nil {
			results = append(results, buildResultItem(knowledge, *personal, states[knowledge.ID]))
		}
	}
	total := int64(len(results))
	if pageResult != nil {
		total = pageResult.Total
	}
	return &MineResult{Results: results, Total: total}, nil
}

// ExpandKnowledge 按 ID 展开详情，并重新按当前用户可读 scope 过滤。
func (s *Service) ExpandKnowledge(ctx context.Context, input ExpandInput) (*ExpandResult, error) {
	if len(input.IDs) == 0 || s.store == nil || s.knowledge == nil {
		return &ExpandResult{Results: []ExpandedItem{}}, nil
	}
	scopes, err := s.store.ListReadableScopes(ctx, input.UserID, true)
	if err != nil {
		return nil, err
	}
	allowed := make(map[string]ReadableScope, len(scopes))
	for _, scope := range scopes {
		allowed[scopeKey(scope.TenantID, scope.KBID)] = scope
	}

	knowledges := make([]*types.Knowledge, 0, len(input.IDs))
	ids := make([]string, 0, len(input.IDs))
	for _, id := range input.IDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		knowledge, err := s.knowledge.GetKnowledgeByIDOnly(ctx, id)
		if err != nil {
			return nil, err
		}
		if knowledge == nil {
			continue
		}
		if _, ok := allowed[scopeKey(knowledge.TenantID, knowledge.KnowledgeBaseID)]; !ok {
			continue
		}
		knowledges = append(knowledges, knowledge)
		ids = append(ids, knowledge.ID)
	}
	states, err := s.store.GetKnowledgeStates(ctx, ids)
	if err != nil {
		return nil, err
	}
	results := make([]ExpandedItem, 0, len(knowledges))
	for _, knowledge := range knowledges {
		scope := allowed[scopeKey(knowledge.TenantID, knowledge.KnowledgeBaseID)]
		results = append(results, buildExpandedItem(knowledge, scope, states[knowledge.ID]))
	}
	return &ExpandResult{Results: results}, nil
}

func buildResultItem(knowledge *types.Knowledge, scope ReadableScope, state *types.WikaKnowledgeState) ResultItem {
	result := ResultItem{
		KnowledgeID: knowledge.ID,
		SourceSpace: scope.Source,
	}
	if scopeAllowsField(scope, "title") {
		result.Title = knowledge.Title
	}
	if scopeAllowsField(scope, "source_tenant_id") {
		result.SourceTenantID = knowledge.TenantID
	}
	if scopeAllowsField(scope, "source_kb_id") {
		result.SourceKBID = knowledge.KnowledgeBaseID
	}
	if scopeAllowsField(scope, "freshness_status") {
		result.FreshnessStatus = "fresh"
	}
	if scopeAllowsField(scope, "updated_at") {
		result.UpdatedAt = knowledge.UpdatedAt
	}
	if scopeAllowsField(scope, "snippet") || scopeAllowsField(scope, "content") {
		result.Snippet = compactSnippet(knowledge)
	}
	if state != nil {
		if scopeAllowsField(scope, "quality_score") {
			result.QualityScore = state.QualityScore
		}
		if scopeAllowsField(scope, "freshness_status") && state.FreshnessStatus != "" {
			result.FreshnessStatus = state.FreshnessStatus
		}
	}
	return result
}

func buildExpandedItem(knowledge *types.Knowledge, scope ReadableScope, state *types.WikaKnowledgeState) ExpandedItem {
	content := strings.TrimSpace(knowledge.Description)
	if meta, err := knowledge.ManualMetadata(); err == nil && meta != nil && strings.TrimSpace(meta.Content) != "" {
		content = strings.TrimSpace(meta.Content)
	}
	item := ExpandedItem{
		KnowledgeID: knowledge.ID,
		SourceSpace: scope.Source,
	}
	if scopeAllowsField(scope, "title") {
		item.Title = knowledge.Title
	}
	if scopeAllowsField(scope, "source_tenant_id") {
		item.SourceTenantID = knowledge.TenantID
	}
	if scopeAllowsField(scope, "source_kb_id") {
		item.SourceKBID = knowledge.KnowledgeBaseID
	}
	if scopeAllowsField(scope, "source") {
		item.Source = knowledge.Source
	}
	if scopeAllowsField(scope, "freshness_status") {
		item.FreshnessStatus = "fresh"
	}
	if scopeAllowsField(scope, "updated_at") {
		item.UpdatedAt = knowledge.UpdatedAt
	}
	if scopeAllowsField(scope, "content") {
		item.Content = content
	}
	if state != nil {
		if scopeAllowsField(scope, "quality_score") {
			item.QualityScore = state.QualityScore
		}
		if scopeAllowsField(scope, "freshness_status") && state.FreshnessStatus != "" {
			item.FreshnessStatus = state.FreshnessStatus
		}
	}
	return item
}

func scopeAllowsField(scope ReadableScope, field string) bool {
	if scope.Source != SourceShared {
		return true
	}
	for _, allowed := range wikaorgshare.SanitizeAllowedFields(scope.AllowedFields) {
		if allowed == field {
			return true
		}
	}
	return false
}

func pageResultKnowledges(pageResult *types.PageResult) []*types.Knowledge {
	if pageResult == nil || pageResult.Data == nil {
		return nil
	}
	switch data := pageResult.Data.(type) {
	case []*types.Knowledge:
		return data
	case []types.Knowledge:
		out := make([]*types.Knowledge, 0, len(data))
		for i := range data {
			out = append(out, &data[i])
		}
		return out
	default:
		return nil
	}
}

func scopeKey(tenantID uint64, kbID string) string {
	return strconv.FormatUint(tenantID, 10) + ":" + kbID
}

func filterReadableScopesByKB(scopes []ReadableScope, kbID string) []ReadableScope {
	filtered := make([]ReadableScope, 0, len(scopes))
	for _, scope := range scopes {
		if scope.KBID == kbID {
			filtered = append(filtered, scope)
		}
	}
	return filtered
}

func compactSnippet(knowledge *types.Knowledge) string {
	text := strings.TrimSpace(knowledge.Description)
	if text == "" {
		if meta, err := knowledge.ManualMetadata(); err == nil && meta != nil {
			text = strings.TrimSpace(meta.Content)
		}
	}
	if text == "" {
		text = strings.TrimSpace(knowledge.Title)
	}
	runes := []rune(text)
	if len(runes) > 240 {
		return string(runes[:240])
	}
	return text
}

func (s *Service) searchGraphBestEffort(ctx context.Context, scopes []ReadableScope, query string, limit int) ([]GraphContribution, bool) {
	if s.graph == nil {
		return nil, false
	}
	contribution, err := s.graph.SearchGraph(ctx, scopes, query, limit)
	if err != nil {
		return nil, true
	}
	return contribution, false
}
