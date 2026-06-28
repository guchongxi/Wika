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
	input *wikafreshness.RunCheckInput
}

func (s *stubWikaFreshnessService) RunCheck(_ context.Context, input wikafreshness.RunCheckInput) (*types.WikaFreshnessCheck, error) {
	s.input = &input
	return &types.WikaFreshnessCheck{ID: 41, TenantID: input.TenantID, KBID: input.KBID, Trigger: input.Trigger, Status: wikafreshness.CheckStatusCompleted}, nil
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
	r.POST("/api/v1/wika/kb/:id/freshness/checks", h.RunCheck)
	return r
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
