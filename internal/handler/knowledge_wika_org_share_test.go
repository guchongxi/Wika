package handler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	wikascope "github.com/Tencent/WeKnora/internal/wika/scope"
)

type stubOrgShareKnowledgeService struct {
	interfaces.KnowledgeService
	knowledge             *types.Knowledge
	getKnowledgeFileCalls int
}

func (s *stubOrgShareKnowledgeService) GetKnowledgeByIDOnly(ctx context.Context, id string) (*types.Knowledge, error) {
	if s.knowledge != nil && s.knowledge.ID == id {
		return s.knowledge, nil
	}
	return nil, nil
}

func (s *stubOrgShareKnowledgeService) GetKnowledgeFile(ctx context.Context, id string) (io.ReadCloser, string, error) {
	s.getKnowledgeFileCalls++
	return io.NopCloser(strings.NewReader("secret file")), "secret.txt", nil
}

type stubOrgShareScopeResolver struct {
	decision wikascope.Decision
}

func (s *stubOrgShareScopeResolver) Resolve(ctx context.Context, actor wikascope.Actor, resource wikascope.Resource, action wikascope.Action) (wikascope.Decision, error) {
	return s.decision, nil
}

type stubOrgShareFeatureGate struct {
	enabled bool
}

func (g stubOrgShareFeatureGate) GetBool(ctx context.Context, key string, envName string, def bool) bool {
	return g.enabled
}

func TestGetKnowledgeUsesWikaOrgShareScopeAndRedactsContent(t *testing.T) {
	service := &stubOrgShareKnowledgeService{knowledge: &types.Knowledge{
		ID:              "k-shared",
		TenantID:        80,
		KnowledgeBaseID: "kb-shared",
		Title:           "共享标题",
		Description:     "不应泄露的共享正文",
		FilePath:        "/secret/file",
		Metadata:        types.JSON(`{"content":"secret"}`),
	}}
	h := newOrgShareKnowledgeHandler(service)
	r := newOrgShareKnowledgeRouter()
	r.GET("/knowledge/:id", h.GetKnowledge)

	req := httptest.NewRequest(http.MethodGet, "/knowledge/k-shared", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "共享标题") {
		t.Fatalf("expected shared title in response, body=%s", body)
	}
	for _, leaked := range []string{"不应泄露的共享正文", "/secret/file", "secret"} {
		if strings.Contains(body, leaked) {
			t.Fatalf("shared direct read leaked %q: %s", leaked, body)
		}
	}
	if service.knowledge.Description == "" {
		t.Fatal("redaction must not mutate the original knowledge entity")
	}
}

func TestGetKnowledgeSharedScopeCanExposeAllowedMetadata(t *testing.T) {
	updatedAt := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	service := &stubOrgShareKnowledgeService{knowledge: &types.Knowledge{
		ID:              "k-shared",
		TenantID:        80,
		KnowledgeBaseID: "kb-shared",
		Title:           "共享标题",
		Description:     "不应泄露的共享正文",
		UpdatedAt:       updatedAt,
		Metadata:        types.JSON(`{"content":"secret"}`),
	}}
	h := newOrgShareKnowledgeHandlerWithFields(service, []string{"id", "title", "source_tenant_id", "source_kb_id", "updated_at"})
	r := newOrgShareKnowledgeRouter()
	r.GET("/knowledge/:id", h.GetKnowledge)

	req := httptest.NewRequest(http.MethodGet, "/knowledge/k-shared", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, expected := range []string{`"id":"k-shared"`, `"tenant_id":80`, `"knowledge_base_id":"kb-shared"`, `"updated_at":"2026-06-29T12:00:00Z"`} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected %q in shared direct read response, body=%s", expected, body)
		}
	}
	for _, leaked := range []string{"不应泄露的共享正文", "secret"} {
		if strings.Contains(body, leaked) {
			t.Fatalf("shared direct read leaked %q: %s", leaked, body)
		}
	}
}

func TestDownloadKnowledgeFileRejectsWikaOrgShareWithoutFileAccess(t *testing.T) {
	service := &stubOrgShareKnowledgeService{knowledge: &types.Knowledge{
		ID:              "k-shared",
		TenantID:        80,
		KnowledgeBaseID: "kb-shared",
		Title:           "共享标题",
	}}
	h := newOrgShareKnowledgeHandler(service)
	r := newOrgShareKnowledgeRouter()
	r.GET("/knowledge/:id/download", h.DownloadKnowledgeFile)

	req := httptest.NewRequest(http.MethodGet, "/knowledge/k-shared/download", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", w.Code, w.Body.String())
	}
	if service.getKnowledgeFileCalls != 0 {
		t.Fatalf("shared download must be blocked before file fetch, got calls=%d", service.getKnowledgeFileCalls)
	}
}

func TestPreviewKnowledgeFileRejectsWikaOrgShareWithoutFileAccess(t *testing.T) {
	service := &stubOrgShareKnowledgeService{knowledge: &types.Knowledge{
		ID:              "k-shared",
		TenantID:        80,
		KnowledgeBaseID: "kb-shared",
		Title:           "共享标题",
	}}
	h := newOrgShareKnowledgeHandler(service)
	r := newOrgShareKnowledgeRouter()
	r.GET("/knowledge/:id/preview", h.PreviewKnowledgeFile)

	req := httptest.NewRequest(http.MethodGet, "/knowledge/k-shared/preview", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", w.Code, w.Body.String())
	}
	if service.getKnowledgeFileCalls != 0 {
		t.Fatalf("shared preview must be blocked before file fetch, got calls=%d", service.getKnowledgeFileCalls)
	}
}

func TestWikaOrgShareIgnoresUnsafeAllowedFieldsFromResolver(t *testing.T) {
	service := &stubOrgShareKnowledgeService{knowledge: &types.Knowledge{
		ID:              "k-shared",
		TenantID:        80,
		KnowledgeBaseID: "kb-shared",
		Title:           "共享标题",
		Description:     "不应泄露的共享正文",
	}}
	h := newOrgShareKnowledgeHandlerWithFields(service, []string{"id", "title", "content", "file"})
	r := newOrgShareKnowledgeRouter()
	r.GET("/knowledge/:id", h.GetKnowledge)
	r.GET("/knowledge/:id/download", h.DownloadKnowledgeFile)

	readReq := httptest.NewRequest(http.MethodGet, "/knowledge/k-shared", nil)
	readW := httptest.NewRecorder()
	r.ServeHTTP(readW, readReq)
	if readW.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", readW.Code, readW.Body.String())
	}
	if strings.Contains(readW.Body.String(), "不应泄露的共享正文") {
		t.Fatalf("unsafe content field leaked direct read body=%s", readW.Body.String())
	}

	downloadReq := httptest.NewRequest(http.MethodGet, "/knowledge/k-shared/download", nil)
	downloadW := httptest.NewRecorder()
	r.ServeHTTP(downloadW, downloadReq)
	if downloadW.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", downloadW.Code, downloadW.Body.String())
	}
	if service.getKnowledgeFileCalls != 0 {
		t.Fatalf("unsafe file field must not call file fetch, got calls=%d", service.getKnowledgeFileCalls)
	}
}

func TestWikaOrgShareRevokedResolverDecisionDeniesDirectRead(t *testing.T) {
	service := &stubOrgShareKnowledgeService{knowledge: &types.Knowledge{
		ID:              "k-shared",
		TenantID:        80,
		KnowledgeBaseID: "kb-shared",
		Title:           "共享标题",
	}}
	h := newOrgShareKnowledgeHandlerWithDecision(service, wikascope.Decision{Allowed: false})
	r := newOrgShareKnowledgeRouter()
	r.GET("/knowledge/:id", h.GetKnowledge)

	req := httptest.NewRequest(http.MethodGet, "/knowledge/k-shared", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestWikaOrgShareFeatureGateDisabledDeniesDirectRead(t *testing.T) {
	service := &stubOrgShareKnowledgeService{knowledge: &types.Knowledge{
		ID:              "k-shared",
		TenantID:        80,
		KnowledgeBaseID: "kb-shared",
		Title:           "共享标题",
	}}
	h := newOrgShareKnowledgeHandler(service)
	h.wikaOrgShareGate = stubOrgShareFeatureGate{enabled: false}
	r := newOrgShareKnowledgeRouter()
	r.GET("/knowledge/:id", h.GetKnowledge)

	req := httptest.NewRequest(http.MethodGet, "/knowledge/k-shared", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", w.Code, w.Body.String())
	}
}

func newOrgShareKnowledgeRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	r.Use(func(c *gin.Context) {
		c.Set(types.UserIDContextKey.String(), "u-target")
		c.Set(types.TenantIDContextKey.String(), uint64(90))
		ctx := context.WithValue(c.Request.Context(), types.UserIDContextKey, "u-target")
		ctx = context.WithValue(ctx, types.TenantIDContextKey, uint64(90))
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	return r
}

func newOrgShareKnowledgeHandler(service *stubOrgShareKnowledgeService) *KnowledgeHandler {
	return newOrgShareKnowledgeHandlerWithFields(service, []string{"id", "title"})
}

func newOrgShareKnowledgeHandlerWithFields(service *stubOrgShareKnowledgeService, allowedFields []string) *KnowledgeHandler {
	return newOrgShareKnowledgeHandlerWithDecision(service, wikascope.Decision{
		Allowed: true,
		Scopes: []wikascope.Scope{{
			TenantID:      80,
			KBID:          "kb-shared",
			Source:        wikascope.ScopeSourceShared,
			AllowedFields: allowedFields,
		}},
	})
}

func newOrgShareKnowledgeHandlerWithDecision(service *stubOrgShareKnowledgeService, decision wikascope.Decision) *KnowledgeHandler {
	h := &KnowledgeHandler{
		kgService:         service,
		wikaScopeResolver: &stubOrgShareScopeResolver{decision: decision},
		wikaOrgShareGate:  stubOrgShareFeatureGate{enabled: true},
	}
	return h
}
