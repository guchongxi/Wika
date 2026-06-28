package search

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

type fakeSearchStore struct {
	scopes        []ReadableScope
	states        map[string]*types.WikaKnowledgeState
	accessRecords []AccessRecord
	includeTeam   bool
}

func (s *fakeSearchStore) ListReadableScopes(ctx context.Context, userID string, includeTeam bool) ([]ReadableScope, error) {
	s.includeTeam = includeTeam
	if includeTeam {
		return s.scopes, nil
	}
	out := make([]ReadableScope, 0, len(s.scopes))
	for _, scope := range s.scopes {
		if scope.Source == SourcePersonal {
			out = append(out, scope)
		}
	}
	return out, nil
}

func (s *fakeSearchStore) GetKnowledgeStates(ctx context.Context, knowledgeIDs []string) (map[string]*types.WikaKnowledgeState, error) {
	return s.states, nil
}

func (s *fakeSearchStore) RecordAccess(ctx context.Context, records []AccessRecord) error {
	s.accessRecords = append(s.accessRecords, records...)
	return nil
}

type fakeKnowledgeSearcher struct {
	scopes []types.KnowledgeSearchScope
	query  string
	limit  int
	resp   []*types.Knowledge
	more   bool

	listKBID string
	listPage *types.Pagination
	listResp *types.PageResult

	byID map[string]*types.Knowledge
}

func (s *fakeKnowledgeSearcher) SearchKnowledgeForScopes(ctx context.Context, scopes []types.KnowledgeSearchScope, keyword string, offset, limit int, fileTypes []string) ([]*types.Knowledge, bool, error) {
	s.scopes = scopes
	s.query = keyword
	s.limit = limit
	return s.resp, s.more, nil
}

func (s *fakeKnowledgeSearcher) ListPagedKnowledgeByKnowledgeBaseID(ctx context.Context, kbID string, page *types.Pagination, filter types.KnowledgeListFilter) (*types.PageResult, error) {
	s.listKBID = kbID
	s.listPage = page
	if s.listResp != nil {
		return s.listResp, nil
	}
	return types.NewPageResult(0, page, []*types.Knowledge{}), nil
}

func (s *fakeKnowledgeSearcher) GetKnowledgeByIDOnly(ctx context.Context, id string) (*types.Knowledge, error) {
	if s.byID != nil {
		return s.byID[id], nil
	}
	return nil, nil
}

func TestSearchKnowledgeUsesReadableScopesAndReturnsCompactResults(t *testing.T) {
	updatedAt := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	store := &fakeSearchStore{
		scopes: []ReadableScope{
			{TenantID: 70, KBID: "kb-personal", Source: SourcePersonal},
			{TenantID: 80, KBID: "kb-team", Source: SourceTeam},
		},
		states: map[string]*types.WikaKnowledgeState{
			"k-personal": {KnowledgeID: "k-personal", QualityScore: 86, FreshnessStatus: "fresh"},
			"k-team":     {KnowledgeID: "k-team", QualityScore: 72, FreshnessStatus: "expiring"},
		},
	}
	searcher := &fakeKnowledgeSearcher{
		resp: []*types.Knowledge{
			{ID: "k-personal", TenantID: 70, KnowledgeBaseID: "kb-personal", Title: "个人排查记录", Description: "日志排查", UpdatedAt: updatedAt},
			{ID: "k-team", TenantID: 80, KnowledgeBaseID: "kb-team", Title: "团队运行手册", Description: "指标排查", UpdatedAt: updatedAt},
		},
		more: true,
	}
	svc := &Service{store: store, knowledge: searcher}

	got, err := svc.SearchKnowledge(context.Background(), SearchInput{
		UserID:      "u-test",
		Query:       "排查",
		Limit:       5,
		IncludeTeam: true,
	})
	if err != nil {
		t.Fatalf("SearchKnowledge returned error: %v", err)
	}
	if !store.includeTeam {
		t.Fatal("expected includeTeam to be passed to scope store")
	}
	if len(searcher.scopes) != 2 || searcher.scopes[0].KBID != "kb-personal" || searcher.scopes[1].KBID != "kb-team" {
		t.Fatalf("unexpected search scopes: %+v", searcher.scopes)
	}
	if searcher.query != "排查" || searcher.limit != 5 {
		t.Fatalf("unexpected search request: query=%q limit=%d", searcher.query, searcher.limit)
	}
	if !got.Truncated || len(got.Results) != 2 {
		t.Fatalf("unexpected search result: %+v", got)
	}
	if got.Results[0].KnowledgeID != "k-personal" ||
		got.Results[0].SourceSpace != SourcePersonal ||
		got.Results[0].QualityScore != 86 ||
		got.Results[0].FreshnessStatus != "fresh" {
		t.Fatalf("unexpected personal result: %+v", got.Results[0])
	}
	if got.Results[1].SourceSpace != SourceTeam || got.Results[1].FreshnessStatus != "expiring" {
		t.Fatalf("unexpected team result: %+v", got.Results[1])
	}
	if len(store.accessRecords) != 2 || store.accessRecords[0].KnowledgeID != "k-personal" {
		t.Fatalf("expected access records for returned results, got %+v", store.accessRecords)
	}
}

func TestSearchKnowledgeCanExcludeTeamScopes(t *testing.T) {
	store := &fakeSearchStore{scopes: []ReadableScope{
		{TenantID: 70, KBID: "kb-personal", Source: SourcePersonal},
		{TenantID: 80, KBID: "kb-team", Source: SourceTeam},
	}}
	searcher := &fakeKnowledgeSearcher{}
	svc := &Service{store: store, knowledge: searcher}

	_, err := svc.SearchKnowledge(context.Background(), SearchInput{
		UserID:      "u-test",
		Query:       "排查",
		IncludeTeam: false,
	})
	if err != nil {
		t.Fatalf("SearchKnowledge returned error: %v", err)
	}
	if len(searcher.scopes) != 1 || searcher.scopes[0].KBID != "kb-personal" {
		t.Fatalf("expected only personal scope, got %+v", searcher.scopes)
	}
}

func TestListMyKnowledgeUsesPersonalDefaultScope(t *testing.T) {
	updatedAt := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	store := &fakeSearchStore{
		scopes: []ReadableScope{
			{TenantID: 70, KBID: "kb-personal", Source: SourcePersonal},
			{TenantID: 80, KBID: "kb-team", Source: SourceTeam},
		},
		states: map[string]*types.WikaKnowledgeState{
			"k-personal": {KnowledgeID: "k-personal", QualityScore: 91, FreshnessStatus: "fresh"},
		},
	}
	searcher := &fakeKnowledgeSearcher{
		listResp: types.NewPageResult(1, &types.Pagination{Page: 1, PageSize: 10}, []*types.Knowledge{
			{ID: "k-personal", TenantID: 70, KnowledgeBaseID: "kb-personal", Title: "个人知识", Description: "个人片段", UpdatedAt: updatedAt},
		}),
	}
	svc := &Service{store: store, knowledge: searcher}

	got, err := svc.ListMyKnowledge(context.Background(), MineInput{
		UserID: "u-test",
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("ListMyKnowledge returned error: %v", err)
	}
	if store.includeTeam {
		t.Fatal("my knowledge must not include team scopes")
	}
	if searcher.listKBID != "kb-personal" {
		t.Fatalf("expected personal default KB, got %q", searcher.listKBID)
	}
	if searcher.listPage == nil || searcher.listPage.PageSize != 10 {
		t.Fatalf("unexpected pagination: %+v", searcher.listPage)
	}
	if got.Total != 1 || len(got.Results) != 1 {
		t.Fatalf("unexpected mine result: %+v", got)
	}
	if got.Results[0].SourceSpace != SourcePersonal || got.Results[0].QualityScore != 91 {
		t.Fatalf("unexpected mine item: %+v", got.Results[0])
	}
}

func TestExpandKnowledgeResultsRechecksReadableScopes(t *testing.T) {
	store := &fakeSearchStore{
		scopes: []ReadableScope{
			{TenantID: 70, KBID: "kb-personal", Source: SourcePersonal},
		},
		states: map[string]*types.WikaKnowledgeState{
			"k-allowed": {KnowledgeID: "k-allowed", QualityScore: 91, FreshnessStatus: "fresh"},
		},
	}
	allowed := &types.Knowledge{
		ID:              "k-allowed",
		TenantID:        70,
		KnowledgeBaseID: "kb-personal",
		Title:           "个人知识",
		Description:     "个人摘要",
		UpdatedAt:       time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC),
	}
	if err := allowed.SetManualMetadata(types.NewManualKnowledgeMetadata("完整正文", types.ManualKnowledgeStatusPublish, 1)); err != nil {
		t.Fatalf("set manual metadata: %v", err)
	}
	searcher := &fakeKnowledgeSearcher{byID: map[string]*types.Knowledge{
		"k-allowed": allowed,
		"k-denied": {
			ID:              "k-denied",
			TenantID:        80,
			KnowledgeBaseID: "kb-team",
			Title:           "不可读知识",
		},
	}}
	svc := &Service{store: store, knowledge: searcher}

	got, err := svc.ExpandKnowledge(context.Background(), ExpandInput{
		UserID: "u-test",
		IDs:    []string{"k-allowed", "k-denied"},
	})
	if err != nil {
		t.Fatalf("ExpandKnowledge returned error: %v", err)
	}
	if len(got.Results) != 1 {
		t.Fatalf("expected only readable result, got %+v", got)
	}
	item := got.Results[0]
	if item.KnowledgeID != "k-allowed" ||
		item.Content != "完整正文" ||
		item.SourceSpace != SourcePersonal ||
		item.QualityScore != 91 {
		t.Fatalf("unexpected expanded item: %+v", item)
	}
}
