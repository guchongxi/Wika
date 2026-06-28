package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

	RegisterWikaRoutes(api, nil, nil, &handler.WikaSuggestionHandler{}, nil, nil, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/wika/suggestions", strings.NewReader(`{"knowledge_id":"k-personal"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code == http.StatusNotFound {
		t.Fatalf("expected suggestions route to be registered, got 404")
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

	RegisterWikaRoutes(api, nil, nil, &handler.WikaSuggestionHandler{}, nil, nil, nil, nil, nil, nil, nil)

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

	RegisterWikaRoutes(api, nil, nil, nil, &handler.WikaEvaluationHandler{}, nil, nil, nil, nil, nil, nil)

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

	RegisterWikaRoutes(api, nil, nil, nil, &handler.WikaEvaluationHandler{}, nil, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/wika/kb/kb-team/eval/runs", strings.NewReader(`{"dataset_id":11}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code == http.StatusNotFound {
		t.Fatalf("expected evaluation run route to be registered, got 404")
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

	RegisterWikaRoutes(api, nil, nil, nil, nil, &handler.WikaFreshnessHandler{}, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/wika/kb/kb-team/freshness/checks", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code == http.StatusNotFound {
		t.Fatalf("expected freshness check route to be registered, got 404")
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

	RegisterWikaRoutes(api, nil, nil, nil, nil, &handler.WikaFreshnessHandler{}, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/wika/freshness/items/7", strings.NewReader(`{"action":"mark_updated"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code == http.StatusNotFound {
		t.Fatalf("expected freshness item route to be registered, got 404")
	}
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

	RegisterWikaRoutes(api, nil, nil, nil, nil, nil, &handler.WikaGraphHandler{}, nil, nil, nil, nil)

	for _, tc := range []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/api/v1/wika/kb/kb-team/graph/overview"},
		{method: http.MethodGet, path: "/api/v1/wika/kb/kb-team/graph/entities"},
		{method: http.MethodGet, path: "/api/v1/wika/kb/kb-team/graph/entities/1"},
		{method: http.MethodGet, path: "/api/v1/wika/kb/kb-team/graph/edges"},
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

	RegisterWikaRoutes(api, nil, nil, nil, nil, nil, nil, &handler.WikaConflictHandler{}, nil, nil, nil)

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

	RegisterWikaRoutes(api, nil, nil, nil, nil, nil, nil, nil, &handler.WikaVersionHandler{}, nil, nil)

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

	RegisterWikaRoutes(api, nil, nil, nil, nil, nil, nil, nil, nil, &handler.WikaURLRefreshHandler{}, nil)

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
