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
	wikaauth "github.com/Tencent/WeKnora/internal/wika/auth"
)

type stubWikaTokenService struct {
	createInput *wikaauth.CreateTokenInput
	createResp  *wikaauth.CreatedToken
	listResp    []*types.WikaUserToken
	revokedUser string
	revokedID   uint64
	usageFilter *wikaauth.TokenUsageFilter
	usageResp   *wikaauth.TokenUsageListResult
	eventFilter *wikaauth.TokenUsageFilter
	eventResp   *wikaauth.TokenUsageEventListResult
}

func (s *stubWikaTokenService) CreateToken(_ context.Context, input wikaauth.CreateTokenInput) (*wikaauth.CreatedToken, error) {
	s.createInput = &input
	if s.createResp != nil {
		return s.createResp, nil
	}
	return &wikaauth.CreatedToken{
		Plaintext: "wika_pat_plaintext",
		Metadata: &types.WikaUserToken{
			ID:          12,
			UserID:      input.UserID,
			TenantID:    input.TenantID,
			Name:        input.Name,
			TokenPrefix: "wika_pat_plainte",
			TokenHash:   "stored-hash-must-not-leak",
			Scopes:      mustJSON(tinyScopes(input.Scopes)),
			ExpiresAt:   input.ExpiresAt,
		},
	}, nil
}

func (s *stubWikaTokenService) ListTokens(_ context.Context, userID string, tenantID uint64) ([]*types.WikaUserToken, error) {
	return s.listResp, nil
}

func (s *stubWikaTokenService) RevokeToken(_ context.Context, userID string, tokenID uint64) error {
	s.revokedUser = userID
	s.revokedID = tokenID
	return nil
}

func (s *stubWikaTokenService) ListUsage(_ context.Context, filter wikaauth.TokenUsageFilter) (*wikaauth.TokenUsageListResult, error) {
	s.usageFilter = &filter
	if s.usageResp != nil {
		return s.usageResp, nil
	}
	return &wikaauth.TokenUsageListResult{
		Scope: filter.Scope,
		Total: 1,
		Items: []wikaauth.TokenUsageItem{{
			Token: wikaauth.TokenUsageToken{
				ID:               12,
				Name:             "Claude Code",
				TokenPrefix:      "wika_pat_plainte",
				OwnerUserID:      "u-test",
				OwnerUsername:    "alice",
				Scopes:           []string{"knowledge:push"},
				Status:           "valid",
				ConnectionStatus: "active",
				ExpiresAt:        time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC),
				TokenHash:        "stored-hash-must-not-leak",
			},
			Summary: wikaauth.TokenUsageSummary{
				TotalCalls:   1,
				SuccessCount: 1,
				Tools: []wikaauth.TokenUsageToolSummary{{
					ToolName:     "push_knowledge",
					APIMethod:    http.MethodPost,
					APIPath:      "/api/v1/wika/knowledge/push",
					SuccessCount: 1,
				}},
			},
		}},
	}, nil
}

func (s *stubWikaTokenService) ListUsageEvents(_ context.Context, filter wikaauth.TokenUsageFilter) (*wikaauth.TokenUsageEventListResult, error) {
	s.eventFilter = &filter
	if s.eventResp != nil {
		return s.eventResp, nil
	}
	return &wikaauth.TokenUsageEventListResult{
		Scope: filter.Scope,
		Total: 1,
		Events: []wikaauth.TokenUsageEventItem{{
			ID:            91,
			TokenID:       12,
			TokenName:     "Claude Code",
			TokenPrefix:   "wika_pat_plainte",
			OwnerUserID:   "u-test",
			OwnerUsername: "alice",
			ToolName:      "push_knowledge",
			APIMethod:     http.MethodPost,
			APIPath:       "/api/v1/wika/knowledge/push",
			StatusCode:    http.StatusOK,
			Success:       true,
			LatencyMS:     123,
			KnowledgeID:   "k-1",
			CreatedAt:     time.Date(2026, 7, 2, 10, 20, 30, 0, time.UTC),
		}},
	}, nil
}

func tinyScopes(scopes []string) []byte {
	b, _ := json.Marshal(scopes)
	return b
}

func mustJSON(b []byte) types.JSON {
	return types.JSON(b)
}

func newWikaTokenTestRouter(service *stubWikaTokenService, now time.Time) *gin.Engine {
	return newWikaTokenTestRouterWithRole(service, now, types.TenantRoleViewer)
}

func newWikaTokenTestRouterWithRole(service *stubWikaTokenService, now time.Time, role types.TenantRole) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	r.Use(func(c *gin.Context) {
		user := &types.User{
			ID:       "u-test",
			TenantID: 7,
			IsActive: true,
		}
		c.Set(types.UserIDContextKey.String(), "u-test")
		c.Set(types.TenantIDContextKey.String(), uint64(7))
		c.Set(types.TenantRoleContextKey.String(), role)
		c.Set(types.UserContextKey.String(), user)
		ctx := context.WithValue(c.Request.Context(), types.UserIDContextKey, "u-test")
		ctx = context.WithValue(ctx, types.TenantIDContextKey, uint64(7))
		ctx = context.WithValue(ctx, types.TenantRoleContextKey, role)
		ctx = context.WithValue(ctx, types.UserContextKey, user)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	h := &WikaTokenHandler{service: service, now: func() time.Time { return now }}
	r.GET("/api/v1/wika/tokens", h.ListTokens)
	r.GET("/api/v1/wika/tokens/usage", h.ListTokenUsage)
	r.GET("/api/v1/wika/tokens/usage/events", h.ListTokenUsageEvents)
	r.GET("/api/v1/wika/mcp/admin/authorize", h.AuthorizeMCPAdminToolset)
	r.POST("/api/v1/wika/tokens", h.CreateToken)
	r.DELETE("/api/v1/wika/tokens/:id", h.RevokeToken)
	return r
}

func doWikaTokenJSON(t *testing.T, r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestWikaTokenUsageMineUsesCurrentUserAndDoesNotExposeSecrets(t *testing.T) {
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	service := &stubWikaTokenService{}
	r := newWikaTokenTestRouter(service, now)

	w := doWikaTokenJSON(t, r, http.MethodGet, "/api/v1/wika/tokens/usage?tool_name=push_knowledge&limit=10", "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if service.usageFilter == nil {
		t.Fatal("expected usage service to be called")
	}
	if service.usageFilter.Scope != wikaauth.UsageScopeMine ||
		service.usageFilter.ViewerUserID != "u-test" ||
		service.usageFilter.TenantID != 7 ||
		service.usageFilter.ToolName != "push_knowledge" ||
		service.usageFilter.Limit != 10 {
		t.Fatalf("unexpected usage filter: %+v", service.usageFilter)
	}
	if strings.Contains(w.Body.String(), "stored-hash") ||
		strings.Contains(w.Body.String(), `"token_hash"`) ||
		strings.Contains(w.Body.String(), `"created_by_ip"`) {
		t.Fatalf("usage response must not expose token secret material: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"connection_status":"active"`) ||
		!strings.Contains(w.Body.String(), `"tool_name":"push_knowledge"`) {
		t.Fatalf("unexpected usage response: %s", w.Body.String())
	}
}

func TestWikaTokenUsageAllRequiresAdminAndSupportsEvents(t *testing.T) {
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	viewerService := &stubWikaTokenService{}
	viewerRouter := newWikaTokenTestRouterWithRole(viewerService, now, types.TenantRoleViewer)

	w := doWikaTokenJSON(t, viewerRouter, http.MethodGet, "/api/v1/wika/tokens/usage?scope=all", "")

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected viewer scope=all forbidden, got %d body=%s", w.Code, w.Body.String())
	}
	if viewerService.usageFilter != nil {
		t.Fatalf("forbidden all scope must not query service: %+v", viewerService.usageFilter)
	}

	adminService := &stubWikaTokenService{}
	adminRouter := newWikaTokenTestRouterWithRole(adminService, now, types.TenantRoleAdmin)
	w = doWikaTokenJSON(t, adminRouter, http.MethodGet, "/api/v1/wika/tokens/usage?scope=all&owner_user_id=u-other&tool_name=push_knowledge", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected admin scope=all 200, got %d body=%s", w.Code, w.Body.String())
	}
	if adminService.usageFilter == nil ||
		adminService.usageFilter.Scope != wikaauth.UsageScopeAll ||
		adminService.usageFilter.OwnerUserID != "u-other" {
		t.Fatalf("unexpected admin usage filter: %+v", adminService.usageFilter)
	}

	w = doWikaTokenJSON(t, adminRouter, http.MethodGet, "/api/v1/wika/tokens/usage/events?scope=all&token_id=12", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected admin events 200, got %d body=%s", w.Code, w.Body.String())
	}
	if adminService.eventFilter == nil ||
		adminService.eventFilter.Scope != wikaauth.UsageScopeAll ||
		adminService.eventFilter.TokenID != 12 {
		t.Fatalf("unexpected admin event filter: %+v", adminService.eventFilter)
	}
	if strings.Contains(w.Body.String(), "stored-hash") || !strings.Contains(w.Body.String(), `"knowledge_id":"k-1"`) {
		t.Fatalf("unexpected events response: %s", w.Body.String())
	}
}

func TestWikaMCPAdminToolsetAuthorizeRequiresTenantAdmin(t *testing.T) {
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	viewerRouter := newWikaTokenTestRouterWithRole(&stubWikaTokenService{}, now, types.TenantRoleViewer)

	w := doWikaTokenJSON(t, viewerRouter, http.MethodGet, "/api/v1/wika/mcp/admin/authorize", "")

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected viewer forbidden, got %d body=%s", w.Code, w.Body.String())
	}

	adminRouter := newWikaTokenTestRouterWithRole(&stubWikaTokenService{}, now, types.TenantRoleAdmin)
	w = doWikaTokenJSON(t, adminRouter, http.MethodGet, "/api/v1/wika/mcp/admin/authorize", "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected admin allowed, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"toolset":"admin"`) ||
		!strings.Contains(w.Body.String(), `"allowed":true`) {
		t.Fatalf("unexpected authorize response: %s", w.Body.String())
	}
}

func TestWikaTokenCreateReturnsPlaintextOnceAndNoHash(t *testing.T) {
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	service := &stubWikaTokenService{}
	r := newWikaTokenTestRouter(service, now)

	body := `{"name":"Claude Code","scopes":["knowledge:push","knowledge:search"],"expires_at":"2026-07-29T12:00:00Z"}`
	w := doWikaTokenJSON(t, r, http.MethodPost, "/api/v1/wika/tokens", body)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", w.Code, w.Body.String())
	}
	if service.createInput == nil {
		t.Fatal("expected CreateToken to be called")
	}
	if service.createInput.UserID != "u-test" || service.createInput.TenantID != 7 {
		t.Fatalf("unexpected token owner: %+v", service.createInput)
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if resp["token"] != "wika_pat_plaintext" {
		t.Fatalf("created response must include plaintext token once, got %v", resp["token"])
	}
	if _, ok := resp["token_hash"]; ok {
		t.Fatalf("created response must not expose token_hash: %s", w.Body.String())
	}
	if strings.Contains(w.Body.String(), "stored-hash") || strings.Contains(w.Body.String(), "pepper") {
		t.Fatalf("created response leaked secret material: %s", w.Body.String())
	}
}

func TestWikaTokenCreateAdminScopeRequiresTenantAdmin(t *testing.T) {
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	body := `{"name":"Admin MCP","scopes":["mcp:admin"],"expires_at":"2026-07-29T12:00:00Z"}`

	viewerService := &stubWikaTokenService{}
	viewerRouter := newWikaTokenTestRouterWithRole(viewerService, now, types.TenantRoleViewer)
	w := doWikaTokenJSON(t, viewerRouter, http.MethodPost, "/api/v1/wika/tokens", body)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected viewer forbidden, got %d body=%s", w.Code, w.Body.String())
	}
	if viewerService.createInput != nil {
		t.Fatalf("forbidden admin scope must not create token: %+v", viewerService.createInput)
	}

	adminService := &stubWikaTokenService{}
	adminRouter := newWikaTokenTestRouterWithRole(adminService, now, types.TenantRoleAdmin)
	w = doWikaTokenJSON(t, adminRouter, http.MethodPost, "/api/v1/wika/tokens", body)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected admin token created, got %d body=%s", w.Code, w.Body.String())
	}
	if adminService.createInput == nil ||
		len(adminService.createInput.Scopes) != 1 ||
		adminService.createInput.Scopes[0] != "mcp:admin" {
		t.Fatalf("unexpected admin token input: %+v", adminService.createInput)
	}
}

func TestWikaTokenListDoesNotExposePlaintextOrHash(t *testing.T) {
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	service := &stubWikaTokenService{
		listResp: []*types.WikaUserToken{{
			ID:          12,
			UserID:      "u-test",
			TenantID:    7,
			Name:        "Claude Code",
			TokenPrefix: "wika_pat_plainte",
			TokenHash:   "stored-hash-must-not-leak",
			Scopes:      types.JSON([]byte(`["knowledge:search"]`)),
			ExpiresAt:   now.Add(24 * time.Hour),
		}},
	}
	r := newWikaTokenTestRouter(service, now)

	w := doWikaTokenJSON(t, r, http.MethodGet, "/api/v1/wika/tokens", "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "stored-hash") || strings.Contains(w.Body.String(), `"token":`) {
		t.Fatalf("list response must not expose plaintext token or hash: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "wika_pat_plainte") {
		t.Fatalf("list response should include token_prefix: %s", w.Body.String())
	}
}

func TestWikaTokenRevokeUsesCurrentUser(t *testing.T) {
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	service := &stubWikaTokenService{}
	r := newWikaTokenTestRouter(service, now)

	w := doWikaTokenJSON(t, r, http.MethodDelete, "/api/v1/wika/tokens/12", "")

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d body=%s", w.Code, w.Body.String())
	}
	if service.revokedUser != "u-test" || service.revokedID != 12 {
		t.Fatalf("unexpected revoke call: user=%q id=%d", service.revokedUser, service.revokedID)
	}
}

func TestWikaTokenCreateRejectsUnsafeRequest(t *testing.T) {
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		body string
	}{
		{
			name: "expires beyond 90 days",
			body: `{"name":"Claude Code","scopes":["knowledge:search"],"expires_at":"2026-09-28T12:00:01Z"}`,
		},
		{
			name: "unknown scope",
			body: `{"name":"Claude Code","scopes":["tenant:admin"],"expires_at":"2026-07-29T12:00:00Z"}`,
		},
		{
			name: "foreign tenant id",
			body: `{"name":"Claude Code","tenant_id":8,"scopes":["knowledge:search"],"expires_at":"2026-07-29T12:00:00Z"}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service := &stubWikaTokenService{}
			r := newWikaTokenTestRouter(service, now)
			w := doWikaTokenJSON(t, r, http.MethodPost, "/api/v1/wika/tokens", tc.body)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
			}
			if service.createInput != nil {
				t.Fatalf("unsafe request must not call service: %+v", service.createInput)
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
		})
	}
}
