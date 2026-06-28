package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
	wikasuggestion "github.com/Tencent/WeKnora/internal/wika/suggestion"
)

type stubWikaSuggestionService struct {
	input *wikasuggestion.CreateInput
	resp  *wikasuggestion.SuggestionResult
}

func (s *stubWikaSuggestionService) CreateSuggestion(_ context.Context, input wikasuggestion.CreateInput) (*wikasuggestion.SuggestionResult, error) {
	s.input = &input
	if s.resp != nil {
		return s.resp, nil
	}
	return &wikasuggestion.SuggestionResult{
		SuggestionID:     99,
		AIDecision:       wikasuggestion.DecisionNeedsConfirmation,
		Status:           wikasuggestion.StatusPendingHuman,
		CorrectedTitle:   "排查记录",
		CorrectedContent: "修正后的团队知识",
		Risks:            []string{"prompt_injection"},
	}, nil
}

func newWikaSuggestionTestRouter(service *stubWikaSuggestionService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	r.Use(func(c *gin.Context) {
		c.Set(types.UserIDContextKey.String(), "u-test")
		c.Set(types.TenantIDContextKey.String(), uint64(80))
		c.Next()
	})
	h := &WikaSuggestionHandler{service: service}
	r.POST("/api/v1/wika/suggestions", h.CreateSuggestion)
	return r
}

func doWikaSuggestionJSON(t *testing.T, r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestWikaSuggestionCreatePassesActorAndTargetToService(t *testing.T) {
	service := &stubWikaSuggestionService{}
	r := newWikaSuggestionTestRouter(service)

	body := `{
		"knowledge_id":"k-personal",
		"target_space_id":81,
		"target_kb_id":"kb-team",
		"reason":"团队可复用",
		"idempotency_key":"idem-1"
	}`
	w := doWikaSuggestionJSON(t, r, http.MethodPost, "/api/v1/wika/suggestions", body)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if service.input == nil {
		t.Fatal("expected CreateSuggestion to be called")
	}
	if service.input.SubmitterID != "u-test" ||
		service.input.KnowledgeID != "k-personal" ||
		service.input.TargetTenantID != 81 ||
		service.input.TargetKBID != "kb-team" ||
		service.input.Reason != "团队可复用" ||
		service.input.IdempotencyKey != "idem-1" {
		t.Fatalf("unexpected input: %+v", service.input)
	}

	var resp struct {
		SuggestionID     uint64   `json:"suggestion_id"`
		AIDecision       string   `json:"ai_decision"`
		Status           string   `json:"status"`
		CorrectedTitle   string   `json:"corrected_title"`
		CorrectedContent string   `json:"corrected_content"`
		Risks            []string `json:"risks"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.SuggestionID != 99 ||
		resp.AIDecision != string(wikasuggestion.DecisionNeedsConfirmation) ||
		resp.Status != string(wikasuggestion.StatusPendingHuman) ||
		resp.CorrectedTitle != "排查记录" ||
		resp.CorrectedContent == "" ||
		len(resp.Risks) != 1 {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestWikaSuggestionCreateDefaultsTargetSpaceToCurrentTenant(t *testing.T) {
	service := &stubWikaSuggestionService{}
	r := newWikaSuggestionTestRouter(service)

	w := doWikaSuggestionJSON(t, r, http.MethodPost, "/api/v1/wika/suggestions", `{"knowledge_id":"k-personal"}`)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if service.input == nil || service.input.TargetTenantID != 80 {
		t.Fatalf("expected current tenant as default target, got %+v", service.input)
	}
}

func TestWikaSuggestionCreateRejectsMissingKnowledgeID(t *testing.T) {
	service := &stubWikaSuggestionService{}
	r := newWikaSuggestionTestRouter(service)

	w := doWikaSuggestionJSON(t, r, http.MethodPost, "/api/v1/wika/suggestions", `{"reason":"x"}`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
	if service.input != nil {
		t.Fatalf("service should not be called: %+v", service.input)
	}
}
