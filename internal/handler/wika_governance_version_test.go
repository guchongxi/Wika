package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
	wikaversion "github.com/Tencent/WeKnora/internal/wika/governance/version"
)

type stubWikaVersionService struct {
	listInput    wikaversion.ListVersionsInput
	diffInput    wikaversion.DiffInput
	restoreInput wikaversion.RestoreInput
}

func (s *stubWikaVersionService) ListVersions(_ context.Context, input wikaversion.ListVersionsInput) ([]*types.WikaKnowledgeVersion, error) {
	s.listInput = input
	return []*types.WikaKnowledgeVersion{{ID: 2, KnowledgeID: input.KnowledgeID, TenantID: input.TenantID, VersionNo: 2, ContentHash: "hash-2"}}, nil
}

func (s *stubWikaVersionService) Diff(_ context.Context, input wikaversion.DiffInput) (*wikaversion.DiffResult, error) {
	s.diffInput = input
	return &wikaversion.DiffResult{FromVersionNo: 1, ToVersionNo: 2, TitleChanged: true, ContentChanged: true}, nil
}

func (s *stubWikaVersionService) Restore(_ context.Context, input wikaversion.RestoreInput) (*wikaversion.RestoreResult, error) {
	s.restoreInput = input
	return &wikaversion.RestoreResult{RestoredFromVersionID: input.VersionID, NewVersionID: 9, KnowledgeID: input.KnowledgeID, Status: "restored"}, nil
}

func newWikaVersionTestRouter(service *stubWikaVersionService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	r.Use(func(c *gin.Context) {
		c.Set(types.UserIDContextKey.String(), "u-reviewer")
		c.Set(types.TenantIDContextKey.String(), uint64(80))
		c.Next()
	})
	h := &WikaVersionHandler{service: service}
	r.GET("/api/v1/wika/knowledge/:id/versions", h.ListVersions)
	r.GET("/api/v1/wika/knowledge/:id/versions/:version_id/diff", h.Diff)
	r.POST("/api/v1/wika/knowledge/:id/versions/:version_id/restore", h.Restore)
	return r
}

func TestWikaVersionListPassesActorTenantAndKnowledge(t *testing.T) {
	service := &stubWikaVersionService{}
	r := newWikaVersionTestRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/wika/knowledge/k-1/versions?limit=10&offset=5", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if service.listInput.ActorID != "u-reviewer" ||
		service.listInput.TenantID != 80 ||
		service.listInput.KnowledgeID != "k-1" ||
		service.listInput.Limit != 10 ||
		service.listInput.Offset != 5 {
		t.Fatalf("unexpected list input: %+v", service.listInput)
	}
}

func TestWikaVersionRestoreParsesReasonAndVersionID(t *testing.T) {
	service := &stubWikaVersionService{}
	r := newWikaVersionTestRouter(service)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/wika/knowledge/k-1/versions/7/restore", strings.NewReader(`{"reason":"误操作恢复"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if service.restoreInput.ActorID != "u-reviewer" ||
		service.restoreInput.TenantID != 80 ||
		service.restoreInput.KnowledgeID != "k-1" ||
		service.restoreInput.VersionID != 7 ||
		service.restoreInput.Reason != "误操作恢复" {
		t.Fatalf("unexpected restore input: %+v", service.restoreInput)
	}
}

func TestWikaVersionDiffParsesVersionIDs(t *testing.T) {
	service := &stubWikaVersionService{}
	r := newWikaVersionTestRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/wika/knowledge/k-1/versions/1/diff?to_version_id=2", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if service.diffInput.ActorID != "u-reviewer" ||
		service.diffInput.TenantID != 80 ||
		service.diffInput.KnowledgeID != "k-1" ||
		service.diffInput.FromVersion != 1 ||
		service.diffInput.ToVersion != 2 {
		t.Fatalf("unexpected diff input: %+v", service.diffInput)
	}
}
