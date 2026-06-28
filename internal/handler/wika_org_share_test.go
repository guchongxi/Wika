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
	wikaorgshare "github.com/Tencent/WeKnora/internal/wika/governance/orgshare"
)

type stubWikaOrgShareService struct {
	createInput *wikaorgshare.CreateShareInput
	createErr   error
	acceptInput *wikaorgshare.AcceptShareInput
	acceptErr   error
	revokeInput *wikaorgshare.RevokeShareInput
	revokeErr   error
}

func (s *stubWikaOrgShareService) CreateShare(_ context.Context, input wikaorgshare.CreateShareInput) (*types.WikaOrgShare, error) {
	s.createInput = &input
	if s.createErr != nil {
		return nil, s.createErr
	}
	return &types.WikaOrgShare{ID: 41, OrgID: input.OrgID, SourceTenantID: input.SourceTenantID, SourceKBID: input.SourceKBID, TargetTenantID: input.TargetTenantID, Status: types.WikaOrgShareStatusPending}, nil
}

func (s *stubWikaOrgShareService) AcceptShare(_ context.Context, input wikaorgshare.AcceptShareInput) (*types.WikaOrgShare, error) {
	s.acceptInput = &input
	if s.acceptErr != nil {
		return nil, s.acceptErr
	}
	return &types.WikaOrgShare{ID: input.ShareID, Status: types.WikaOrgShareStatusActive, AcceptedBy: input.ActorID}, nil
}

func (s *stubWikaOrgShareService) RevokeShare(_ context.Context, input wikaorgshare.RevokeShareInput) (*types.WikaOrgShare, error) {
	s.revokeInput = &input
	if s.revokeErr != nil {
		return nil, s.revokeErr
	}
	return &types.WikaOrgShare{ID: input.ShareID, Status: types.WikaOrgShareStatusRevoked, RevokedBy: input.ActorID}, nil
}

func newWikaOrgShareTestRouter(service *stubWikaOrgShareService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	r.Use(func(c *gin.Context) {
		c.Set(types.UserIDContextKey.String(), "u-test")
		c.Set(types.TenantIDContextKey.String(), uint64(80))
		c.Next()
	})
	h := &WikaOrgShareHandler{service: service}
	r.POST("/api/v1/wika/orgs/:org_id/shares", h.CreateShare)
	r.PUT("/api/v1/wika/orgs/:org_id/shares/:share_id/accept", h.AcceptShare)
	r.DELETE("/api/v1/wika/orgs/:org_id/shares/:share_id", h.RevokeShare)
	return r
}

func TestWikaOrgShareCreatePassesCurrentTenantAsSource(t *testing.T) {
	service := &stubWikaOrgShareService{}
	r := newWikaOrgShareTestRouter(service)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/wika/orgs/org-1/shares", bytes.NewBufferString(`{"source_kb_id":"kb-source","target_tenant_id":90,"allowed_fields":["id","title"]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if service.createInput == nil ||
		service.createInput.ActorID != "u-test" ||
		service.createInput.OrgID != "org-1" ||
		service.createInput.SourceTenantID != 80 ||
		service.createInput.SourceKBID != "kb-source" ||
		service.createInput.TargetTenantID != 90 ||
		len(service.createInput.AllowedFields) != 2 {
		t.Fatalf("unexpected create input: %+v", service.createInput)
	}
}

func TestWikaOrgShareUnsafeAllowedFieldsReturnsBadRequest(t *testing.T) {
	service := &stubWikaOrgShareService{createErr: wikaorgshare.ErrInvalidAllowedFields}
	r := newWikaOrgShareTestRouter(service)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/wika/orgs/org-1/shares", bytes.NewBufferString(`{"source_kb_id":"kb-source","target_tenant_id":90,"allowed_fields":["content"]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestWikaOrgShareAcceptAndRevokePassShareID(t *testing.T) {
	service := &stubWikaOrgShareService{}
	r := newWikaOrgShareTestRouter(service)

	for _, tc := range []struct {
		method string
		path   string
	}{
		{method: http.MethodPut, path: "/api/v1/wika/orgs/org-1/shares/41/accept"},
		{method: http.MethodDelete, path: "/api/v1/wika/orgs/org-1/shares/41"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, bytes.NewBufferString(`{}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for %s %s, got %d body=%s", tc.method, tc.path, w.Code, w.Body.String())
		}
	}
	if service.acceptInput == nil || service.acceptInput.ActorID != "u-test" || service.acceptInput.ShareID != 41 {
		t.Fatalf("unexpected accept input: %+v", service.acceptInput)
	}
	if service.revokeInput == nil || service.revokeInput.ActorID != "u-test" || service.revokeInput.ShareID != 41 {
		t.Fatalf("unexpected revoke input: %+v", service.revokeInput)
	}
}
