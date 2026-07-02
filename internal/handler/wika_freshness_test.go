package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
	wikafreshness "github.com/Tencent/WeKnora/internal/wika/freshness"
)

type stubWikaFreshnessService struct {
	input       *wikafreshness.RunCheckInput
	handleInput *wikafreshness.HandleItemInput
	overviewIn  *wikafreshness.ListInput
}

func (s *stubWikaFreshnessService) RunCheck(_ context.Context, input wikafreshness.RunCheckInput) (*types.WikaFreshnessCheck, error) {
	s.input = &input
	return &types.WikaFreshnessCheck{ID: 41, TenantID: input.TenantID, KBID: input.KBID, Trigger: input.Trigger, Status: wikafreshness.CheckStatusCompleted}, nil
}

func (s *stubWikaFreshnessService) ListChecks(_ context.Context, input wikafreshness.ListInput) ([]*types.WikaFreshnessCheck, error) {
	return []*types.WikaFreshnessCheck{{ID: 41, TenantID: input.TenantID, KBID: input.KBID}}, nil
}

func (s *stubWikaFreshnessService) ListItems(_ context.Context, input wikafreshness.ListInput) ([]*types.WikaFreshnessCheckItem, error) {
	return []*types.WikaFreshnessCheckItem{{ID: 7, TenantID: input.TenantID, KBID: input.KBID, Status: input.Status}}, nil
}

func (s *stubWikaFreshnessService) Overview(_ context.Context, input wikafreshness.ListInput) (*wikafreshness.OverviewResult, error) {
	s.overviewIn = &input
	return &wikafreshness.OverviewResult{
		LatestCheck: &types.WikaFreshnessCheck{ID: 41, TenantID: input.TenantID, KBID: input.KBID},
		TotalOpen:   2,
		IssueCounts: map[string]int{wikafreshness.IssueExpired: 1, wikafreshness.IssueStale: 1},
	}, nil
}

func (s *stubWikaFreshnessService) HandleItem(_ context.Context, input wikafreshness.HandleItemInput) (*types.WikaFreshnessCheckItem, error) {
	s.handleInput = &input
	return &types.WikaFreshnessCheckItem{ID: input.ItemID, TenantID: input.TenantID, Status: wikafreshness.ItemStatusResolved, ResolutionAction: input.Action, ResolutionNote: input.Note}, nil
}

func newWikaFreshnessTestRouter(service *stubWikaFreshnessService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	r.Use(func(c *gin.Context) {
		c.Set(types.UserIDContextKey.String(), "u-test")
		c.Set(types.TenantIDContextKey.String(), uint64(80))
		c.Next()
	})
	h := &WikaFreshnessHandler{service: service}
	r.GET("/api/v1/wika/kb/:id/freshness/overview", h.Overview)
	r.POST("/api/v1/wika/kb/:id/freshness/checks", h.RunCheck)
	r.PUT("/api/v1/wika/freshness/items/:id", h.HandleItem)
	return r
}

func TestWikaFreshnessOverviewPassesTenantKBToService(t *testing.T) {
	service := &stubWikaFreshnessService{}
	r := newWikaFreshnessTestRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/wika/kb/kb-team/freshness/overview", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if service.overviewIn == nil || service.overviewIn.TenantID != 80 || service.overviewIn.KBID != "kb-team" {
		t.Fatalf("unexpected overview input: %+v", service.overviewIn)
	}
}

func TestWikaFreshnessRunCheckPassesTenantKBToService(t *testing.T) {
	service := &stubWikaFreshnessService{}
	r := newWikaFreshnessTestRouter(service)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/wika/kb/kb-team/freshness/checks", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if service.input == nil || service.input.TenantID != 80 || service.input.KBID != "kb-team" || service.input.Trigger != "manual" {
		t.Fatalf("unexpected freshness input: %+v", service.input)
	}
}

func TestWikaFreshnessHandleItemPassesActorAndActionToService(t *testing.T) {
	service := &stubWikaFreshnessService{}
	r := newWikaFreshnessTestRouter(service)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/wika/freshness/items/7", bytes.NewBufferString(`{"action":"mark_updated","note":"已更新"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if service.handleInput == nil ||
		service.handleInput.ActorID != "u-test" ||
		service.handleInput.TenantID != 80 ||
		service.handleInput.ItemID != 7 ||
		service.handleInput.Action != wikafreshness.ActionMarkUpdated ||
		service.handleInput.Note != "已更新" {
		t.Fatalf("unexpected handle input: %+v", service.handleInput)
	}
}
