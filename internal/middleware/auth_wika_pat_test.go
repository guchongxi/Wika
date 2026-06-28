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

type stubAuthTenantService struct {
	interfaces.TenantService
	tenant *types.Tenant
}

func (s *stubAuthTenantService) GetTenantByID(ctx context.Context, id uint64) (*types.Tenant, error) {
	return s.tenant, nil
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
}

func (s *stubWikaPATVerifier) VerifyToken(ctx context.Context, plaintext, requiredScope string) (*types.WikaUserToken, error) {
	s.calls++
	s.requiredScope = requiredScope
	if s.err != nil {
		return nil, s.err
	}
	return s.token, nil
}

func newWikaPATAuthRouter(t *testing.T, verifier *stubWikaPATVerifier, userSvc *stubAuthUserService) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	tenantSvc := &stubAuthTenantService{tenant: &types.Tenant{ID: 7, Name: "team"}}
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
		c.Status(http.StatusOK)
	})
	r.POST("/api/v1/wika/suggestions", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	r.GET("/api/v1/wika/tokens", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	return r
}

func TestAuthAcceptsWikaPATForDailyKnowledgeRoute(t *testing.T) {
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

func TestAuthRejectsWikaPATOutsideDailyRoutes(t *testing.T) {
	verifier := &stubWikaPATVerifier{token: &types.WikaUserToken{UserID: "u-pat", TenantID: 7}}
	userSvc := &stubAuthUserService{user: &types.User{ID: "u-pat", TenantID: 7, IsActive: true}}
	r := newWikaPATAuthRouter(t, verifier, userSvc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/wika/tokens", nil)
	req.Header.Set("Authorization", "Bearer wika_pat_valid")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected PAT forbidden outside daily routes, got %d body=%s", w.Code, w.Body.String())
	}
	if verifier.calls != 0 {
		t.Fatalf("disallowed route must not call verifier, calls=%d", verifier.calls)
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
