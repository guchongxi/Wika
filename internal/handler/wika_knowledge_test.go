package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
	wikaintake "github.com/Tencent/WeKnora/internal/wika/intake"
	wikasearch "github.com/Tencent/WeKnora/internal/wika/search"
)

type stubWikaIntakeService struct {
	input *wikaintake.PushKnowledgeInput
	resp  *wikaintake.PushKnowledgeResult
}

func (s *stubWikaIntakeService) PushKnowledge(_ context.Context, input wikaintake.PushKnowledgeInput) (*wikaintake.PushKnowledgeResult, error) {
	s.input = &input
	if s.resp != nil {
		return s.resp, nil
	}
	return &wikaintake.PushKnowledgeResult{
		KnowledgeID:  "k-1",
		QualityScore: 86,
		Status:       "created",
		Normalized: wikaintake.NormalizedKnowledge{
			Title:   input.Title,
			Content: input.Content,
			Source:  input.Source,
			Tags:    input.Tags,
		},
		DuplicateCandidates: []wikaintake.DuplicateCandidate{{
			KnowledgeID: "dup-1",
			Title:       "相似知识",
			Score:       0.81,
			Reason:      "标题和正文相似",
		}},
	}, nil
}

type stubWikaSearchService struct {
	input       *wikasearch.SearchInput
	mineInput   *wikasearch.MineInput
	expandInput *wikasearch.ExpandInput
	resp        *wikasearch.SearchResult
	mineResp    *wikasearch.MineResult
	expandResp  *wikasearch.ExpandResult
}

func (s *stubWikaSearchService) SearchKnowledge(_ context.Context, input wikasearch.SearchInput) (*wikasearch.SearchResult, error) {
	s.input = &input
	if s.resp != nil {
		return s.resp, nil
	}
	return &wikasearch.SearchResult{Results: []wikasearch.ResultItem{{
		KnowledgeID:     "k-1",
		Title:           "排查记录",
		Snippet:         "先看日志",
		SourceSpace:     wikasearch.SourcePersonal,
		QualityScore:    86,
		FreshnessStatus: "fresh",
	}}}, nil
}

func (s *stubWikaSearchService) ListMyKnowledge(_ context.Context, input wikasearch.MineInput) (*wikasearch.MineResult, error) {
	s.mineInput = &input
	if s.mineResp != nil {
		return s.mineResp, nil
	}
	return &wikasearch.MineResult{Total: 1, Results: []wikasearch.ResultItem{{
		KnowledgeID:     "k-1",
		Title:           "个人知识",
		Snippet:         "个人片段",
		SourceSpace:     wikasearch.SourcePersonal,
		QualityScore:    91,
		FreshnessStatus: "fresh",
	}}}, nil
}

func (s *stubWikaSearchService) ExpandKnowledge(_ context.Context, input wikasearch.ExpandInput) (*wikasearch.ExpandResult, error) {
	s.expandInput = &input
	if s.expandResp != nil {
		return s.expandResp, nil
	}
	return &wikasearch.ExpandResult{Results: []wikasearch.ExpandedItem{{
		KnowledgeID:     "k-1",
		Title:           "个人知识",
		Content:         "完整正文",
		SourceSpace:     wikasearch.SourcePersonal,
		QualityScore:    91,
		FreshnessStatus: "fresh",
	}}}, nil
}

func newWikaKnowledgeTestRouter(service *stubWikaIntakeService) *gin.Engine {
	return newWikaKnowledgeSearchTestRouter(service, nil)
}

func newWikaKnowledgeSearchTestRouter(intake *stubWikaIntakeService, search *stubWikaSearchService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	r.Use(func(c *gin.Context) {
		c.Set(types.UserIDContextKey.String(), "u-test")
		c.Set(types.TenantIDContextKey.String(), uint64(7))
		c.Next()
	})
	h := &WikaKnowledgeHandler{intake: intake, search: search}
	r.POST("/api/v1/wika/knowledge/push", h.PushKnowledge)
	r.POST("/api/v1/wika/knowledge/search", h.SearchKnowledge)
	r.POST("/api/v1/wika/knowledge/expand", h.ExpandKnowledge)
	r.GET("/api/v1/wika/knowledge/mine", h.ListMyKnowledge)
	return r
}

func doWikaKnowledgeJSON(t *testing.T, r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestWikaKnowledgePushPassesCurrentUserAndDraftToService(t *testing.T) {
	expiresAt := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	service := &stubWikaIntakeService{}
	r := newWikaKnowledgeTestRouter(service)

	body := `{
		"title":"排查记录",
		"content":"先看日志，再看指标。",
		"source":"incident-42",
		"tags":["debug","runbook"],
		"evidence":"线上日志片段",
		"expires_at":"2026-07-29T12:00:00Z",
		"idempotency_key":"idem-1",
		"dry_run":true
	}`
	w := doWikaKnowledgeJSON(t, r, http.MethodPost, "/api/v1/wika/knowledge/push", body)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if service.input == nil {
		t.Fatal("expected PushKnowledge to be called")
	}
	if service.input.UserID != "u-test" || service.input.TenantID != 7 {
		t.Fatalf("unexpected actor in input: %+v", service.input)
	}
	if service.input.Title != "排查记录" ||
		service.input.Content != "先看日志，再看指标。" ||
		service.input.Source != "incident-42" ||
		service.input.Evidence != "线上日志片段" ||
		service.input.IdempotencyKey != "idem-1" ||
		!service.input.DryRun {
		t.Fatalf("unexpected draft input: %+v", service.input)
	}
	if len(service.input.Tags) != 2 || service.input.Tags[0] != "debug" || service.input.Tags[1] != "runbook" {
		t.Fatalf("unexpected tags: %#v", service.input.Tags)
	}
	if service.input.ExpiresAt == nil || !service.input.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("unexpected expires_at: %#v", service.input.ExpiresAt)
	}

	var resp struct {
		KnowledgeID         string                          `json:"knowledge_id"`
		Normalized          wikaintake.NormalizedKnowledge  `json:"normalized"`
		QualityScore        int                             `json:"quality_score"`
		DuplicateCandidates []wikaintake.DuplicateCandidate `json:"duplicate_candidates"`
		Status              string                          `json:"status"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if resp.KnowledgeID != "k-1" || resp.QualityScore != 86 || resp.Status != "created" {
		t.Fatalf("unexpected response: %+v body=%s", resp, w.Body.String())
	}
	if resp.Normalized.Title != "排查记录" || len(resp.DuplicateCandidates) != 1 {
		t.Fatalf("unexpected normalized or duplicates: %+v body=%s", resp, w.Body.String())
	}
}

func TestWikaKnowledgePushRejectsEmptyContentBeforeService(t *testing.T) {
	service := &stubWikaIntakeService{}
	r := newWikaKnowledgeTestRouter(service)

	w := doWikaKnowledgeJSON(t, r, http.MethodPost, "/api/v1/wika/knowledge/push", `{"title":"空内容","content":"   "}`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
	if service.input != nil {
		t.Fatalf("empty content must not call service: %+v", service.input)
	}
	var envelope struct {
		Error struct {
			Code apperrors.ErrorCode `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("expected error envelope JSON: %v", err)
	}
	if envelope.Error.Code != apperrors.ErrBadRequest && envelope.Error.Code != apperrors.ErrValidation {
		t.Fatalf("expected bad request style error, got body=%s", w.Body.String())
	}
}

func TestWikaKnowledgeSearchPassesCurrentUserAndQueryToService(t *testing.T) {
	search := &stubWikaSearchService{}
	r := newWikaKnowledgeSearchTestRouter(nil, search)

	w := doWikaKnowledgeJSON(t, r, http.MethodPost, "/api/v1/wika/knowledge/search", `{"query":"排查","limit":2,"include_team":true,"format":"compact"}`)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if search.input == nil {
		t.Fatal("expected SearchKnowledge to be called")
	}
	if search.input.UserID != "u-test" || search.input.TenantID != 7 {
		t.Fatalf("unexpected actor in input: %+v", search.input)
	}
	if search.input.Query != "排查" || search.input.Limit != 2 || !search.input.IncludeTeam || search.input.Format != "compact" {
		t.Fatalf("unexpected search input: %+v", search.input)
	}
	if !strings.Contains(w.Body.String(), `"source_space":"personal"`) ||
		!strings.Contains(w.Body.String(), `"quality_score":86`) {
		t.Fatalf("unexpected search response: %s", w.Body.String())
	}
}

func TestWikaKnowledgeSearchRejectsEmptyQueryBeforeService(t *testing.T) {
	search := &stubWikaSearchService{}
	r := newWikaKnowledgeSearchTestRouter(nil, search)

	w := doWikaKnowledgeJSON(t, r, http.MethodPost, "/api/v1/wika/knowledge/search", `{"query":"   "}`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
	if search.input != nil {
		t.Fatalf("empty query must not call service: %+v", search.input)
	}
}

func TestWikaKnowledgeMinePassesCurrentUserAndFiltersToService(t *testing.T) {
	search := &stubWikaSearchService{}
	r := newWikaKnowledgeSearchTestRouter(nil, search)

	w := doWikaKnowledgeJSON(t, r, http.MethodGet, "/api/v1/wika/knowledge/mine?limit=10&status=fresh&tag=debug", "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if search.mineInput == nil {
		t.Fatal("expected ListMyKnowledge to be called")
	}
	if search.mineInput.UserID != "u-test" ||
		search.mineInput.Limit != 10 ||
		search.mineInput.Status != "fresh" ||
		search.mineInput.Tag != "debug" {
		t.Fatalf("unexpected mine input: %+v", search.mineInput)
	}
	if !strings.Contains(w.Body.String(), `"total":1`) ||
		!strings.Contains(w.Body.String(), `"source_space":"personal"`) {
		t.Fatalf("unexpected mine response: %s", w.Body.String())
	}
}

func TestWikaKnowledgeExpandPassesIDsToService(t *testing.T) {
	search := &stubWikaSearchService{}
	r := newWikaKnowledgeSearchTestRouter(nil, search)

	w := doWikaKnowledgeJSON(t, r, http.MethodPost, "/api/v1/wika/knowledge/expand", `{"ids":["k-1","k-2"]}`)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if search.expandInput == nil {
		t.Fatal("expected ExpandKnowledge to be called")
	}
	if search.expandInput.UserID != "u-test" || len(search.expandInput.IDs) != 2 || search.expandInput.IDs[1] != "k-2" {
		t.Fatalf("unexpected expand input: %+v", search.expandInput)
	}
	if !strings.Contains(w.Body.String(), `"content":"完整正文"`) ||
		!strings.Contains(w.Body.String(), `"source_space":"personal"`) {
		t.Fatalf("unexpected expand response: %s", w.Body.String())
	}
}

func TestWikaKnowledgeExpandRejectsEmptyIDsBeforeService(t *testing.T) {
	search := &stubWikaSearchService{}
	r := newWikaKnowledgeSearchTestRouter(nil, search)

	w := doWikaKnowledgeJSON(t, r, http.MethodPost, "/api/v1/wika/knowledge/expand", `{"ids":[]}`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
	if search.expandInput != nil {
		t.Fatalf("empty ids must not call service: %+v", search.expandInput)
	}
}
