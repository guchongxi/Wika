package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	wikaauth "github.com/Tencent/WeKnora/internal/wika/auth"
)

type stubAuthUserService struct {
	interfaces.UserService
	user               *types.User
	validateTokenCalls int
}

func (s *stubAuthUserService) ValidateToken(ctx context.Context, token string) (*types.User, uint64, error) {
	s.validateTokenCalls++
	return nil, 0, wikaauth.ErrTokenInvalid
}

func (s *stubAuthUserService) GetUserByID(ctx context.Context, id string) (*types.User, error) {
	return s.user, nil
}

func (s *stubAuthUserService) GetUserByTenantID(ctx context.Context, tenantID uint64) (*types.User, error) {
	return s.user, nil
}

type stubAuthTenantService struct {
	interfaces.TenantService
	tenant *types.Tenant
}

func (s *stubAuthTenantService) GetTenantByID(ctx context.Context, id uint64) (*types.Tenant, error) {
	return s.tenant, nil
}

func (s *stubAuthTenantService) ExtractTenantIDFromAPIKey(apiKey string) (uint64, error) {
	if s.tenant != nil && apiKey == s.tenant.APIKey {
		return s.tenant.ID, nil
	}
	return 0, wikaauth.ErrTokenInvalid
}

type stubAuthMemberService struct {
	interfaces.TenantMemberService
	member *types.TenantMember
}

func (s *stubAuthMemberService) GetMembership(ctx context.Context, userID string, tenantID uint64) (*types.TenantMember, error) {
	return s.member, nil
}

func (s *stubAuthMemberService) HasAnyMembers(ctx context.Context, tenantID uint64) (bool, error) {
	return true, nil
}

type stubWikaPATVerifier struct {
	token         *types.WikaUserToken
	err           error
	requiredScope string
	calls         int
	records       []wikaauth.TokenUsageRecord
}

func (s *stubWikaPATVerifier) VerifyToken(ctx context.Context, plaintext, requiredScope string) (*types.WikaUserToken, error) {
	s.calls++
	s.requiredScope = requiredScope
	if s.err != nil {
		return nil, s.err
	}
	return s.token, nil
}

func (s *stubWikaPATVerifier) RecordUsage(ctx context.Context, record wikaauth.TokenUsageRecord) error {
	s.records = append(s.records, record)
	return nil
}

func newWikaPATAuthRouter(t *testing.T, verifier *stubWikaPATVerifier, userSvc *stubAuthUserService) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	tenantSvc := &stubAuthTenantService{tenant: &types.Tenant{ID: 7, Name: "team", APIKey: "sk-tenant-test"}}
	memberSvc := &stubAuthMemberService{member: &types.TenantMember{
		UserID:   "u-pat",
		TenantID: 7,
		Role:     types.TenantRoleViewer,
		Status:   types.TenantMemberStatusActive,
	}}
	cfg := &config.Config{Tenant: &config.TenantConfig{}}
	r.Use(Auth(tenantSvc, userSvc, memberSvc, cfg, verifier))
	r.POST("/api/v1/wika/knowledge/search", func(c *gin.Context) {
		if got := c.GetString(types.UserIDContextKey.String()); got != "u-pat" {
			t.Fatalf("expected user context from PAT, got %q", got)
		}
		if got := c.GetUint64(types.TenantIDContextKey.String()); got != 7 {
			t.Fatalf("expected tenant context from PAT, got %d", got)
		}
		if got := types.TenantRoleFromContext(c.Request.Context()); got != types.TenantRoleViewer {
			t.Fatalf("expected viewer role from membership, got %q", got)
		}
		if types.IsSystemAdminFromContext(c.Request.Context()) {
			t.Fatal("PAT must not grant SystemAdmin context")
		}
		rawUsage, ok := c.Get(types.WikaPATUsageContextKey.String())
		if !ok {
			t.Fatal("expected Wika PAT usage context")
		}
		usage, ok := rawUsage.(types.WikaPATUsageContext)
		if !ok {
			t.Fatalf("unexpected usage context type: %T", rawUsage)
		}
		if usage.TokenID != 42 ||
			usage.TokenPrefix != "wika_pat_valid" ||
			usage.RequiredScope != "knowledge:search" ||
			usage.ToolName != "search_knowledge" ||
			usage.APIMethod != http.MethodPost ||
			usage.APIPath != "/api/v1/wika/knowledge/search" {
			t.Fatalf("unexpected usage context: %+v", usage)
		}
		c.Status(http.StatusOK)
	})
	r.POST("/api/v1/wika/suggestions", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	r.GET("/api/v1/wika/tokens", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	r.GET("/api/v1/wika/mcp/admin/authorize", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	r.GET("/api/v1/wika/knowledge/mine", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	return r
}

func TestAuthAcceptsWikaPATForDailyKnowledgeRoute(t *testing.T) {
	verifier := &stubWikaPATVerifier{token: &types.WikaUserToken{
		ID:          42,
		UserID:      "u-pat",
		TenantID:    7,
		TokenPrefix: "wika_pat_valid",
		ExpiresAt:   time.Now().Add(time.Hour),
	}}
	userSvc := &stubAuthUserService{user: &types.User{
		ID:       "u-pat",
		TenantID: 7,
		IsActive: true,
	}}
	r := newWikaPATAuthRouter(t, verifier, userSvc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/wika/knowledge/search", nil)
	req.Header.Set("Authorization", "Bearer wika_pat_valid")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected PAT auth to pass, got %d body=%s", w.Code, w.Body.String())
	}
	if verifier.requiredScope != "knowledge:search" {
		t.Fatalf("expected knowledge:search scope, got %q", verifier.requiredScope)
	}
	if userSvc.validateTokenCalls != 0 {
		t.Fatalf("wika_pat token must not be sent through JWT validation")
	}
}

func TestAuthAcceptsWikaPATForMCPAdminAuthorizeRouteWithAdminScope(t *testing.T) {
	verifier := &stubWikaPATVerifier{token: &types.WikaUserToken{
		ID:          42,
		UserID:      "u-pat",
		TenantID:    7,
		TokenPrefix: "wika_pat_valid",
		ExpiresAt:   time.Now().Add(time.Hour),
	}}
	userSvc := &stubAuthUserService{user: &types.User{
		ID:       "u-pat",
		TenantID: 7,
		IsActive: true,
	}}
	r := newWikaPATAuthRouter(t, verifier, userSvc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/wika/mcp/admin/authorize", nil)
	req.Header.Set("Authorization", "Bearer wika_pat_valid")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected admin authorize route to pass auth, got %d body=%s", w.Code, w.Body.String())
	}
	if verifier.requiredScope != "mcp:admin" {
		t.Fatalf("expected mcp:admin scope, got %q", verifier.requiredScope)
	}
}

type stubWikaPATUsageRecorder struct {
	records []wikaauth.TokenUsageRecord
}

func (s *stubWikaPATUsageRecorder) RecordUsage(_ context.Context, record wikaauth.TokenUsageRecord) error {
	s.records = append(s.records, record)
	return nil
}

func TestWikaPATUsageRecorderRecordsCompletedDailyRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := &stubWikaPATUsageRecorder{}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		usage := types.WikaPATUsageContext{
			TokenID:       42,
			TokenPrefix:   "wika_pat_valid",
			UserID:        "u-pat",
			TenantID:      7,
			RequiredScope: "knowledge:push",
			ToolName:      "push_knowledge",
			APIMethod:     http.MethodPost,
			APIPath:       "/api/v1/wika/knowledge/push",
		}
		c.Set(types.WikaPATUsageContextKey.String(), usage)
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), types.WikaPATUsageContextKey, usage))
		c.Next()
	})
	r.Use(WikaPATUsageRecorder(recorder))
	r.POST("/api/v1/wika/knowledge/push", func(c *gin.Context) {
		c.Set(types.WikaKnowledgeIDContextKey.String(), "knowledge-1")
		c.JSON(http.StatusCreated, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/wika/knowledge/push", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected handler status, got %d", w.Code)
	}
	if len(recorder.records) != 1 {
		t.Fatalf("expected one usage record, got %d", len(recorder.records))
	}
	record := recorder.records[0]
	if record.TokenID != 42 ||
		record.UserID != "u-pat" ||
		record.TenantID != 7 ||
		record.ToolName != "push_knowledge" ||
		record.StatusCode != http.StatusCreated ||
		!record.Success ||
		record.KnowledgeID != "knowledge-1" {
		t.Fatalf("unexpected usage record: %+v", record)
	}
}

func TestAuthRecordsUsageAfterCompletedWikaPATDailyRoute(t *testing.T) {
	verifier := &stubWikaPATVerifier{token: &types.WikaUserToken{
		ID:          42,
		UserID:      "u-pat",
		TenantID:    7,
		TokenPrefix: "wika_pat_valid",
		ExpiresAt:   time.Now().Add(time.Hour),
	}}
	userSvc := &stubAuthUserService{user: &types.User{
		ID:       "u-pat",
		TenantID: 7,
		IsActive: true,
	}}
	r := newWikaPATAuthRouter(t, verifier, userSvc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/wika/knowledge/search", nil)
	req.Header.Set("Authorization", "Bearer wika_pat_valid")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected PAT auth to pass, got %d body=%s", w.Code, w.Body.String())
	}
	if len(verifier.records) != 1 {
		t.Fatalf("expected auth to record one usage event, got %d", len(verifier.records))
	}
	record := verifier.records[0]
	if record.TokenID != 42 ||
		record.UserID != "u-pat" ||
		record.TenantID != 7 ||
		record.ToolName != "search_knowledge" ||
		record.StatusCode != http.StatusOK ||
		!record.Success {
		t.Fatalf("unexpected usage record: %+v", record)
	}
}

func TestAuthRequiresSuggestionCreateScopeForWikaPAT(t *testing.T) {
	verifier := &stubWikaPATVerifier{token: &types.WikaUserToken{
		UserID:    "u-pat",
		TenantID:  7,
		ExpiresAt: time.Now().Add(time.Hour),
	}}
	userSvc := &stubAuthUserService{user: &types.User{
		ID:       "u-pat",
		TenantID: 7,
		IsActive: true,
	}}
	r := newWikaPATAuthRouter(t, verifier, userSvc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/wika/suggestions", nil)
	req.Header.Set("Authorization", "Bearer wika_pat_valid")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected PAT auth to pass, got %d body=%s", w.Code, w.Body.String())
	}
	if verifier.requiredScope != "suggestion:create" {
		t.Fatalf("expected suggestion:create scope, got %q", verifier.requiredScope)
	}
}

func TestAuthAcceptsMCPAdminScopeForNonDailyRoutes(t *testing.T) {
	verifier := &stubWikaPATVerifier{token: &types.WikaUserToken{UserID: "u-pat", TenantID: 7}}
	userSvc := &stubAuthUserService{user: &types.User{ID: "u-pat", TenantID: 7, IsActive: true}}
	r := newWikaPATAuthRouter(t, verifier, userSvc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/wika/tokens", nil)
	req.Header.Set("Authorization", "Bearer wika_pat_valid")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected mcp:admin PAT auth to pass non-daily route, got %d body=%s", w.Code, w.Body.String())
	}
	if verifier.requiredScope != "mcp:admin" {
		t.Fatalf("expected mcp:admin scope, got %q", verifier.requiredScope)
	}
}

func TestAuthRejectsWikaPATWithoutRequiredScope(t *testing.T) {
	verifier := &stubWikaPATVerifier{err: wikaauth.ErrTokenScopeDenied}
	userSvc := &stubAuthUserService{user: &types.User{ID: "u-pat", TenantID: 7, IsActive: true}}
	r := newWikaPATAuthRouter(t, verifier, userSvc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/wika/knowledge/search", nil)
	req.Header.Set("Authorization", "Bearer wika_pat_without_search")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected missing scope to be forbidden, got %d body=%s", w.Code, w.Body.String())
	}
	if verifier.requiredScope != "knowledge:search" {
		t.Fatalf("expected knowledge:search scope, got %q", verifier.requiredScope)
	}
}

func TestAuthRejectsTenantAPIKeyForWikaDailyRoutes(t *testing.T) {
	verifier := &stubWikaPATVerifier{}
	userSvc := &stubAuthUserService{user: &types.User{ID: "u-pat", TenantID: 7, IsActive: true}}
	r := newWikaPATAuthRouter(t, verifier, userSvc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/wika/knowledge/mine", nil)
	req.Header.Set("X-API-Key", "sk-tenant-test")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected tenant API key to be rejected for Wika daily route, got %d body=%s", w.Code, w.Body.String())
	}
	if verifier.calls != 0 {
		t.Fatalf("tenant API key must not invoke PAT verifier, calls=%d", verifier.calls)
	}
}
