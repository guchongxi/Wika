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

	RegisterWikaRoutes(api, nil, nil, &handler.WikaSuggestionHandler{}, nil, nil)

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

	RegisterWikaRoutes(api, nil, nil, &handler.WikaSuggestionHandler{}, nil, nil)

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

	RegisterWikaRoutes(api, nil, nil, nil, &handler.WikaEvaluationHandler{}, nil)

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

	RegisterWikaRoutes(api, nil, nil, nil, &handler.WikaEvaluationHandler{}, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/wika/kb/kb-team/eval/runs", strings.NewReader(`{"dataset_id":11}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code == http.StatusNotFound {
		t.Fatalf("expected evaluation run route to be registered, got 404")
	}
}
