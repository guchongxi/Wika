package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/config"
	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
)

func TestRegisterWikaRoutesIncludesSuggestions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(middleware.ErrorHandler())
	engine.Use(func(c *gin.Context) {
		c.Set(types.UserIDContextKey.String(), "u-test")
		c.Set(types.TenantIDContextKey.String(), uint64(80))
		c.Next()
	})
	api := engine.Group("/api/v1")

	RegisterWikaRoutes(api, nil, nil, &handler.WikaSuggestionHandler{}, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/wika/suggestions", strings.NewReader(`{"knowledge_id":"k-personal"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code == http.StatusNotFound {
		t.Fatalf("expected suggestions route to be registered, got 404")
	}
}

func TestRegisterWikaRoutesIncludesTokenUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(middleware.ErrorHandler())
	engine.Use(func(c *gin.Context) {
		c.Set(types.UserIDContextKey.String(), "u-test")
		c.Set(types.TenantIDContextKey.String(), uint64(80))
		c.Next()
	})
	api := engine.Group("/api/v1")

	RegisterWikaRoutes(api, &handler.WikaTokenHandler{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	for _, path := range []string{
		"/api/v1/wika/tokens/usage",
		"/api/v1/wika/tokens/usage/events",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code == http.StatusNotFound {
			t.Fatalf("expected %s to be registered, got 404", path)
		}
	}
}

func TestRegisterWikaRoutesIncludesSuggestionReviewAndApply(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(middleware.ErrorHandler())
	engine.Use(func(c *gin.Context) {
		c.Set(types.UserIDContextKey.String(), "u-test")
		c.Set(types.TenantIDContextKey.String(), uint64(80))
		c.Next()
	})
	api := engine.Group("/api/v1")

	RegisterWikaRoutes(api, nil, nil, &handler.WikaSuggestionHandler{}, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	for _, tc := range []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodPut, path: "/api/v1/wika/suggestions/99/human-review", body: `{"final_decision":"approved"}`},
		{method: http.MethodPost, path: "/api/v1/wika/suggestions/99/apply", body: `{}`},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code == http.StatusNotFound {
			t.Fatalf("expected %s %s to be registered, got 404", tc.method, tc.path)
		}
	}
}

func TestRegisterWikaRoutesIncludesEvaluationDatasets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(middleware.ErrorHandler())
	engine.Use(func(c *gin.Context) {
		c.Set(types.UserIDContextKey.String(), "u-test")
		c.Set(types.TenantIDContextKey.String(), uint64(80))
		c.Next()
	})
	api := engine.Group("/api/v1")

	RegisterWikaRoutes(api, nil, nil, nil, &handler.WikaEvaluationHandler{}, nil, nil, nil, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/wika/kb/kb-team/eval/datasets", strings.NewReader(`{"name":"团队检索黄金 QA"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code == http.StatusNotFound {
		t.Fatalf("expected evaluation dataset route to be registered, got 404")
	}
}

func TestRegisterWikaRoutesIncludesEvaluationRuns(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(middleware.ErrorHandler())
	engine.Use(func(c *gin.Context) {
		c.Set(types.UserIDContextKey.String(), "u-test")
		c.Set(types.TenantIDContextKey.String(), uint64(80))
		c.Next()
	})
	api := engine.Group("/api/v1")

	RegisterWikaRoutes(api, nil, nil, nil, &handler.WikaEvaluationHandler{}, nil, nil, nil, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/wika/kb/kb-team/eval/runs", strings.NewReader(`{"dataset_id":11}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code == http.StatusNotFound {
		t.Fatalf("expected evaluation run route to be registered, got 404")
	}
}

func TestRegisterWikaRoutesIncludesEvaluationExportAndTrend(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(middleware.ErrorHandler())
	engine.Use(func(c *gin.Context) {
		c.Set(types.UserIDContextKey.String(), "u-test")
		c.Set(types.TenantIDContextKey.String(), uint64(80))
		c.Next()
	})
	api := engine.Group("/api/v1")

	RegisterWikaRoutes(api, nil, nil, nil, &handler.WikaEvaluationHandler{}, nil, nil, nil, nil, nil, nil, nil, nil)

	for _, tc := range []struct {
		method string
		path   string
	}{
		{method: http.MethodPost, path: "/api/v1/wika/kb/kb-team/eval/datasets/11/import"},
		{method: http.MethodGet, path: "/api/v1/wika/kb/kb-team/eval/datasets/11/export"},
		{method: http.MethodGet, path: "/api/v1/wika/kb/kb-team/eval/trend"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code == http.StatusNotFound {
			t.Fatalf("expected %s %s to be registered, got 404", tc.method, tc.path)
		}
	}
}

func TestRegisterWikaRoutesIncludesFreshnessChecks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(middleware.ErrorHandler())
	engine.Use(func(c *gin.Context) {
		c.Set(types.UserIDContextKey.String(), "u-test")
		c.Set(types.TenantIDContextKey.String(), uint64(80))
		c.Next()
	})
	api := engine.Group("/api/v1")

	RegisterWikaRoutes(api, nil, nil, nil, nil, &handler.WikaFreshnessHandler{}, nil, nil, nil, nil, nil, nil, nil)

	for _, tc := range []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodGet, path: "/api/v1/wika/kb/kb-team/freshness/overview"},
		{method: http.MethodPost, path: "/api/v1/wika/kb/kb-team/freshness/checks", body: `{}`},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code == http.StatusNotFound {
			t.Fatalf("expected %s %s to be registered, got 404", tc.method, tc.path)
		}
	}
}

func TestRegisterWikaRoutesIncludesFreshnessItems(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(middleware.ErrorHandler())
	engine.Use(func(c *gin.Context) {
		c.Set(types.UserIDContextKey.String(), "u-test")
		c.Set(types.TenantIDContextKey.String(), uint64(80))
		c.Next()
	})
	api := engine.Group("/api/v1")

	RegisterWikaRoutes(api, nil, nil, nil, nil, &handler.WikaFreshnessHandler{}, nil, nil, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/wika/freshness/items/7", strings.NewReader(`{"action":"mark_updated"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code == http.StatusNotFound {
		t.Fatalf("expected freshness item route to be registered, got 404")
	}
}

func TestRegisterWikaFreshnessMutationsRequireAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enforceRBAC := true
	engine := gin.New()
	engine.Use(middleware.ErrorHandler())
	engine.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, types.UserIDContextKey, "u-contributor")
		ctx = context.WithValue(ctx, types.TenantIDContextKey, uint64(80))
		ctx = context.WithValue(ctx, types.TenantRoleContextKey, types.TenantRoleContributor)
		c.Request = c.Request.WithContext(ctx)
		c.Set(types.UserIDContextKey.String(), "u-contributor")
		c.Set(types.TenantIDContextKey.String(), uint64(80))
		c.Next()
	})
	api := engine.Group("/api/v1")
	guards := &rbacGuards{cfg: &config.Config{Tenant: &config.TenantConfig{EnableRBAC: &enforceRBAC}}}

	RegisterWikaRoutes(api, nil, nil, nil, nil, &handler.WikaFreshnessHandler{}, nil, nil, nil, nil, nil, nil, guards)

	for _, tc := range []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodPost, path: "/api/v1/wika/kb/kb-team/freshness/checks", body: `{}`},
		{method: http.MethodPut, path: "/api/v1/wika/freshness/items/7", body: `{"action":"mark_updated"}`},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Fatalf("Contributor must not mutate freshness via %s %s, got %d", tc.method, tc.path, w.Code)
		}
	}
}

func TestRegisterWikaGovernanceMutationsRequireDocumentedRoles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enforceRBAC := true
	guards := &rbacGuards{cfg: &config.Config{Tenant: &config.TenantConfig{EnableRBAC: &enforceRBAC}}}

	for _, tc := range []struct {
		name   string
		role   types.TenantRole
		method string
		path   string
		body   string
	}{
		{
			name:   "viewer cannot create conflict check",
			role:   types.TenantRoleViewer,
			method: http.MethodPost,
			path:   "/api/v1/wika/kb/kb-team/conflicts/checks",
			body:   `{"trigger":"manual"}`,
		},
		{
			name:   "contributor cannot resolve conflict",
			role:   types.TenantRoleContributor,
			method: http.MethodPut,
			path:   "/api/v1/wika/conflicts/7",
			body:   `{"status":"resolved"}`,
		},
		{
			name:   "contributor cannot restore version",
			role:   types.TenantRoleContributor,
			method: http.MethodPost,
			path:   "/api/v1/wika/knowledge/k-1/versions/1/restore",
			body:   `{"reason":"误操作恢复"}`,
		},
		{
			name:   "viewer cannot create url refresh job",
			role:   types.TenantRoleViewer,
			method: http.MethodPost,
			path:   "/api/v1/wika/knowledge/k-1/url-refresh",
			body:   `{"source_url":"https://example.com/doc"}`,
		},
		{
			name:   "contributor cannot update url refresh schedule",
			role:   types.TenantRoleContributor,
			method: http.MethodPut,
			path:   "/api/v1/wika/knowledge/k-1/url-refresh/schedules/21",
			body:   `{"enabled":true,"cron_expr":"0 * * * *"}`,
		},
		{
			name:   "contributor cannot disable url refresh schedule",
			role:   types.TenantRoleContributor,
			method: http.MethodDelete,
			path:   "/api/v1/wika/knowledge/k-1/url-refresh/schedules/21",
			body:   `{}`,
		},
		{
			name:   "contributor cannot review url refresh job",
			role:   types.TenantRoleContributor,
			method: http.MethodPut,
			path:   "/api/v1/wika/url-refresh/11/review",
			body:   `{"decision":"apply"}`,
		},
		{
			name:   "viewer cannot create eval schedule",
			role:   types.TenantRoleViewer,
			method: http.MethodPost,
			path:   "/api/v1/wika/kb/kb-team/eval/schedules",
			body:   `{"dataset_id":11,"cron_expr":"0 * * * *","enabled":true}`,
		},
		{
			name:   "contributor cannot update eval schedule",
			role:   types.TenantRoleContributor,
			method: http.MethodPut,
			path:   "/api/v1/wika/kb/kb-team/eval/schedules/31",
			body:   `{"cron_expr":"0 3 * * *","enabled":false}`,
		},
		{
			name:   "contributor cannot disable eval schedule",
			role:   types.TenantRoleContributor,
			method: http.MethodDelete,
			path:   "/api/v1/wika/kb/kb-team/eval/schedules/31",
			body:   `{}`,
		},
		{
			name:   "viewer cannot create org share",
			role:   types.TenantRoleViewer,
			method: http.MethodPost,
			path:   "/api/v1/wika/orgs/org-1/shares",
			body:   `{"source_kb_id":"kb-source","target_tenant_id":90,"allowed_fields":["id","title"]}`,
		},
		{
			name:   "contributor cannot accept org share",
			role:   types.TenantRoleContributor,
			method: http.MethodPut,
			path:   "/api/v1/wika/orgs/org-1/shares/41/accept",
			body:   `{}`,
		},
		{
			name:   "contributor cannot revoke org share",
			role:   types.TenantRoleContributor,
			method: http.MethodDelete,
			path:   "/api/v1/wika/orgs/org-1/shares/41",
			body:   `{}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine := newWikaGovernanceRBACRouter(tc.role, guards)
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, req)
			if w.Code != http.StatusForbidden {
				t.Fatalf("expected 403 for %s %s as %s, got %d", tc.method, tc.path, tc.role, w.Code)
			}
		})
	}
}

func TestRegisterWikaCoreMutationsRequireDocumentedRoles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enforceRBAC := true
	guards := &rbacGuards{cfg: &config.Config{Tenant: &config.TenantConfig{EnableRBAC: &enforceRBAC}}}

	for _, tc := range []struct {
		name   string
		role   types.TenantRole
		method string
		path   string
		body   string
	}{
		{
			name:   "contributor cannot human-review team suggestion",
			role:   types.TenantRoleContributor,
			method: http.MethodPut,
			path:   "/api/v1/wika/suggestions/99/human-review",
			body:   `{"final_decision":"approved"}`,
		},
		{
			name:   "contributor cannot apply team suggestion",
			role:   types.TenantRoleContributor,
			method: http.MethodPost,
			path:   "/api/v1/wika/suggestions/99/apply",
			body:   `{}`,
		},
		{
			name:   "viewer cannot create evaluation dataset",
			role:   types.TenantRoleViewer,
			method: http.MethodPost,
			path:   "/api/v1/wika/kb/kb-team/eval/datasets",
			body:   `{"name":"团队黄金 QA"}`,
		},
		{
			name:   "contributor cannot import evaluation dataset",
			role:   types.TenantRoleContributor,
			method: http.MethodPost,
			path:   "/api/v1/wika/kb/kb-team/eval/datasets/11/import",
			body:   `{}`,
		},
		{
			name:   "contributor cannot export evaluation dataset",
			role:   types.TenantRoleContributor,
			method: http.MethodGet,
			path:   "/api/v1/wika/kb/kb-team/eval/datasets/11/export",
			body:   ``,
		},
		{
			name:   "viewer cannot dry-run evaluation",
			role:   types.TenantRoleViewer,
			method: http.MethodPost,
			path:   "/api/v1/wika/kb/kb-team/eval/dry-run",
			body:   `{"question":"验证问题"}`,
		},
		{
			name:   "contributor cannot create formal evaluation run",
			role:   types.TenantRoleContributor,
			method: http.MethodPost,
			path:   "/api/v1/wika/kb/kb-team/eval/runs",
			body:   `{"dataset_id":11}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine := newWikaCoreRBACRouter(tc.role, guards)
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, req)
			if w.Code != http.StatusForbidden {
				t.Fatalf("expected 403 for %s %s as %s, got %d", tc.method, tc.path, tc.role, w.Code)
			}
		})
	}
}

func newWikaCoreRBACRouter(role types.TenantRole, guards *rbacGuards) *gin.Engine {
	engine := gin.New()
	engine.Use(middleware.ErrorHandler())
	engine.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, types.UserIDContextKey, "u-core")
		ctx = context.WithValue(ctx, types.TenantIDContextKey, uint64(80))
		ctx = context.WithValue(ctx, types.TenantRoleContextKey, role)
		c.Request = c.Request.WithContext(ctx)
		c.Set(types.UserIDContextKey.String(), "u-core")
		c.Set(types.TenantIDContextKey.String(), uint64(80))
		c.Next()
	})
	api := engine.Group("/api/v1")
	RegisterWikaRoutes(
		api,
		nil,
		nil,
		&handler.WikaSuggestionHandler{},
		&handler.WikaEvaluationHandler{},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		guards,
	)
	return engine
}

func newWikaGovernanceRBACRouter(role types.TenantRole, guards *rbacGuards) *gin.Engine {
	engine := gin.New()
	engine.Use(middleware.ErrorHandler())
	engine.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, types.UserIDContextKey, "u-governance")
		ctx = context.WithValue(ctx, types.TenantIDContextKey, uint64(80))
		ctx = context.WithValue(ctx, types.TenantRoleContextKey, role)
		c.Request = c.Request.WithContext(ctx)
		c.Set(types.UserIDContextKey.String(), "u-governance")
		c.Set(types.TenantIDContextKey.String(), uint64(80))
		c.Next()
	})
	api := engine.Group("/api/v1")
	RegisterWikaRoutes(
		api,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		&handler.WikaConflictHandler{},
		&handler.WikaVersionHandler{},
		&handler.WikaURLRefreshHandler{},
		&handler.WikaEvalScheduleHandler{},
		&handler.WikaOrgShareHandler{},
		guards,
	)
	return engine
}

func TestRegisterWikaRoutesIncludesGraphReadModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(middleware.ErrorHandler())
	engine.Use(func(c *gin.Context) {
		c.Set(types.UserIDContextKey.String(), "u-test")
		c.Set(types.TenantIDContextKey.String(), uint64(80))
		c.Next()
	})
	api := engine.Group("/api/v1")

	RegisterWikaRoutes(api, nil, nil, nil, nil, nil, &handler.WikaGraphHandler{}, nil, nil, nil, nil, nil, nil)

	for _, tc := range []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/api/v1/wika/kb/kb-team/graph/overview"},
		{method: http.MethodGet, path: "/api/v1/wika/kb/kb-team/graph/entities"},
		{method: http.MethodGet, path: "/api/v1/wika/kb/kb-team/graph/entities/1"},
		{method: http.MethodGet, path: "/api/v1/wika/kb/kb-team/graph/edges"},
		{method: http.MethodPost, path: "/api/v1/wika/kb/kb-team/graph/search"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code == http.StatusNotFound {
			t.Fatalf("expected graph route %s %s to be registered, got 404", tc.method, tc.path)
		}
	}
}

func TestRegisterWikaRoutesIncludesConflictGovernance(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(middleware.ErrorHandler())
	engine.Use(func(c *gin.Context) {
		c.Set(types.UserIDContextKey.String(), "u-test")
		c.Set(types.TenantIDContextKey.String(), uint64(80))
		c.Next()
	})
	api := engine.Group("/api/v1")

	RegisterWikaRoutes(api, nil, nil, nil, nil, nil, nil, &handler.WikaConflictHandler{}, nil, nil, nil, nil, nil)

	for _, tc := range []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodGet, path: "/api/v1/wika/kb/kb-team/conflicts", body: ""},
		{method: http.MethodPost, path: "/api/v1/wika/kb/kb-team/conflicts/checks", body: `{"trigger":"manual"}`},
		{method: http.MethodPut, path: "/api/v1/wika/conflicts/7", body: `{"status":"resolved"}`},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code == http.StatusNotFound {
			t.Fatalf("expected %s %s to be registered, got 404", tc.method, tc.path)
		}
	}
}

func TestRegisterWikaRoutesIncludesVersionGovernance(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(middleware.ErrorHandler())
	engine.Use(func(c *gin.Context) {
		c.Set(types.UserIDContextKey.String(), "u-test")
		c.Set(types.TenantIDContextKey.String(), uint64(80))
		c.Next()
	})
	api := engine.Group("/api/v1")

	RegisterWikaRoutes(api, nil, nil, nil, nil, nil, nil, nil, &handler.WikaVersionHandler{}, nil, nil, nil, nil)

	for _, tc := range []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodGet, path: "/api/v1/wika/knowledge/k-1/versions"},
		{method: http.MethodGet, path: "/api/v1/wika/knowledge/k-1/versions/1/diff?to_version_id=2"},
		{method: http.MethodPost, path: "/api/v1/wika/knowledge/k-1/versions/1/restore", body: `{"reason":"误操作恢复"}`},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code == http.StatusNotFound {
			t.Fatalf("expected %s %s to be registered, got 404", tc.method, tc.path)
		}
	}
}

func TestRegisterWikaRoutesIncludesURLRefreshGovernance(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(middleware.ErrorHandler())
	engine.Use(func(c *gin.Context) {
		c.Set(types.UserIDContextKey.String(), "u-test")
		c.Set(types.TenantIDContextKey.String(), uint64(80))
		c.Next()
	})
	api := engine.Group("/api/v1")

	RegisterWikaRoutes(api, nil, nil, nil, nil, nil, nil, nil, nil, &handler.WikaURLRefreshHandler{}, nil, nil, nil)

	for _, tc := range []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodPost, path: "/api/v1/wika/knowledge/k-1/url-refresh", body: `{"source_url":"https://example.com/doc"}`},
		{method: http.MethodPut, path: "/api/v1/wika/knowledge/k-1/url-refresh/schedules/21", body: `{"enabled":true,"cron_expr":"0 * * * *"}`},
		{method: http.MethodDelete, path: "/api/v1/wika/knowledge/k-1/url-refresh/schedules/21", body: `{}`},
		{method: http.MethodPut, path: "/api/v1/wika/url-refresh/11/review", body: `{"decision":"apply"}`},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code == http.StatusNotFound {
			t.Fatalf("expected %s %s to be registered, got 404", tc.method, tc.path)
		}
	}
}

func TestRegisterWikaRoutesIncludesEvalScheduleGovernance(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(middleware.ErrorHandler())
	engine.Use(func(c *gin.Context) {
		c.Set(types.UserIDContextKey.String(), "u-test")
		c.Set(types.TenantIDContextKey.String(), uint64(80))
		c.Next()
	})
	api := engine.Group("/api/v1")

	RegisterWikaRoutes(api, nil, nil, nil, nil, nil, nil, nil, nil, nil, &handler.WikaEvalScheduleHandler{}, nil, nil)

	for _, tc := range []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodPost, path: "/api/v1/wika/kb/kb-team/eval/schedules", body: `{"dataset_id":11,"cron_expr":"0 * * * *","enabled":true}`},
		{method: http.MethodPut, path: "/api/v1/wika/kb/kb-team/eval/schedules/31", body: `{"cron_expr":"0 3 * * *","enabled":false}`},
		{method: http.MethodDelete, path: "/api/v1/wika/kb/kb-team/eval/schedules/31", body: `{}`},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code == http.StatusNotFound {
			t.Fatalf("expected %s %s to be registered, got 404", tc.method, tc.path)
		}
	}
}

func TestRegisterWikaRoutesIncludesOrgShareGovernance(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(middleware.ErrorHandler())
	engine.Use(func(c *gin.Context) {
		c.Set(types.UserIDContextKey.String(), "u-test")
		c.Set(types.TenantIDContextKey.String(), uint64(80))
		c.Next()
	})
	api := engine.Group("/api/v1")

	RegisterWikaRoutes(api, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, &handler.WikaOrgShareHandler{}, nil)

	for _, tc := range []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodPost, path: "/api/v1/wika/orgs/org-1/shares", body: `{"source_kb_id":"kb-source","target_tenant_id":90,"allowed_fields":["id","title"]}`},
		{method: http.MethodPut, path: "/api/v1/wika/orgs/org-1/shares/41/accept", body: `{}`},
		{method: http.MethodDelete, path: "/api/v1/wika/orgs/org-1/shares/41", body: `{}`},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code == http.StatusNotFound {
			t.Fatalf("expected %s %s to be registered, got 404", tc.method, tc.path)
		}
	}
}
