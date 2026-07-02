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
	wikaconflict "github.com/Tencent/WeKnora/internal/wika/governance/conflict"
)

type stubWikaConflictService struct {
	createInput  *wikaconflict.CreateCheckInput
	listInput    *wikaconflict.ListItemsInput
	resolveInput *wikaconflict.ResolveItemInput
	createErr    error
	listErr      error
	resolveErr   error
}

func (s *stubWikaConflictService) CreateCheck(_ context.Context, input wikaconflict.CreateCheckInput) (*types.WikaConflictCheck, error) {
	s.createInput = &input
	if s.createErr != nil {
		return nil, s.createErr
	}
	return &types.WikaConflictCheck{ID: 41, TenantID: input.TenantID, KBID: input.KBID, Trigger: input.Trigger, Status: wikaconflict.CheckStatusPending}, nil
}

func (s *stubWikaConflictService) ListItems(_ context.Context, input wikaconflict.ListItemsInput) ([]*types.WikaConflictItem, int64, error) {
	s.listInput = &input
	if s.listErr != nil {
		return nil, 0, s.listErr
	}
	return []*types.WikaConflictItem{{ID: 7, TenantID: input.TenantID, KBID: input.KBID, Status: input.Status, ConflictType: wikaconflict.ConflictTypeContradiction}}, 1, nil
}

func (s *stubWikaConflictService) ResolveItem(_ context.Context, input wikaconflict.ResolveItemInput) (*types.WikaConflictItem, error) {
	s.resolveInput = &input
	if s.resolveErr != nil {
		return nil, s.resolveErr
	}
	return &types.WikaConflictItem{ID: input.ItemID, TenantID: input.TenantID, Status: input.Status, ReviewerComment: input.Comment, ResolvedBy: input.ActorID}, nil
}

func newWikaConflictTestRouter(service *stubWikaConflictService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	r.Use(func(c *gin.Context) {
		c.Set(types.UserIDContextKey.String(), "u-reviewer")
		c.Set(types.TenantIDContextKey.String(), uint64(80))
		c.Next()
	})
	h := &WikaConflictHandler{service: service}
	r.POST("/api/v1/wika/kb/:id/conflicts/checks", h.CreateCheck)
	r.GET("/api/v1/wika/kb/:id/conflicts", h.ListItems)
	r.PUT("/api/v1/wika/conflicts/:id", h.ResolveItem)
	return r
}

func TestWikaConflictCreateCheckPassesActorTenantAndKB(t *testing.T) {
	service := &stubWikaConflictService{}
	r := newWikaConflictTestRouter(service)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/wika/kb/kb-team/conflicts/checks", bytes.NewBufferString(`{"trigger":"manual"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if service.createInput == nil ||
		service.createInput.ActorID != "u-reviewer" ||
		service.createInput.TenantID != 80 ||
		service.createInput.KBID != "kb-team" ||
		service.createInput.Trigger != wikaconflict.TriggerManual {
		t.Fatalf("unexpected create input: %+v", service.createInput)
	}
}

func TestWikaConflictResolveTerminalTransitionReturnsConflict(t *testing.T) {
	service := &stubWikaConflictService{resolveErr: wikaconflict.ErrConflictItemTerminal}
	r := newWikaConflictTestRouter(service)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/wika/conflicts/7", bytes.NewBufferString(`{"status":"dismissed","comment":"改成误报"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d body=%s", w.Code, w.Body.String())
	}
	if service.resolveInput == nil ||
		service.resolveInput.ActorID != "u-reviewer" ||
		service.resolveInput.TenantID != 80 ||
		service.resolveInput.ItemID != 7 ||
		service.resolveInput.Status != wikaconflict.ItemStatusDismissed ||
		service.resolveInput.Comment != "改成误报" {
		t.Fatalf("unexpected resolve input: %+v", service.resolveInput)
	}
}

func TestWikaConflictFeatureDisabledReturnsNotFound(t *testing.T) {
	t.Run("create", func(t *testing.T) {
		service := &stubWikaConflictService{createErr: wikaconflict.ErrFeatureDisabled}
		r := newWikaConflictTestRouter(service)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/wika/kb/kb-team/conflicts/checks", bytes.NewBufferString(`{"trigger":"manual"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("list", func(t *testing.T) {
		service := &stubWikaConflictService{listErr: wikaconflict.ErrFeatureDisabled}
		r := newWikaConflictTestRouter(service)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/wika/kb/kb-team/conflicts", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("resolve", func(t *testing.T) {
		service := &stubWikaConflictService{resolveErr: wikaconflict.ErrFeatureDisabled}
		r := newWikaConflictTestRouter(service)

		req := httptest.NewRequest(http.MethodPut, "/api/v1/wika/conflicts/7", bytes.NewBufferString(`{"status":"resolved"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d body=%s", w.Code, w.Body.String())
		}
	})
}
