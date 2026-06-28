package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
	wikagraph "github.com/Tencent/WeKnora/internal/wika/graph"
)

type stubWikaGraphService struct {
	entityInput *wikagraph.ListEntitiesInput
	detailInput *wikagraph.GetEntityInput
}

func (s *stubWikaGraphService) Overview(_ context.Context, input wikagraph.OverviewInput) (*wikagraph.Overview, error) {
	return &wikagraph.Overview{TenantID: input.TenantID, KBID: input.KBID, EntityCount: 2, EdgeCount: 1}, nil
}

func (s *stubWikaGraphService) ListEntities(_ context.Context, input wikagraph.ListEntitiesInput) ([]*types.WikaGraphEntity, int64, error) {
	s.entityInput = &input
	return []*types.WikaGraphEntity{{ID: 1, TenantID: input.TenantID, KBID: input.KBID, Name: "索引延迟", EntityType: "concept"}}, 1, nil
}

func (s *stubWikaGraphService) GetEntity(_ context.Context, input wikagraph.GetEntityInput) (*types.WikaGraphEntity, error) {
	s.detailInput = &input
	return &types.WikaGraphEntity{ID: input.EntityID, TenantID: input.TenantID, KBID: input.KBID, Name: "索引延迟", EntityType: "concept"}, nil
}

func (s *stubWikaGraphService) ListEdges(_ context.Context, input wikagraph.ListEdgesInput) ([]*types.WikaGraphEdge, int64, error) {
	return []*types.WikaGraphEdge{{ID: 11, TenantID: input.TenantID, KBID: input.KBID, RelationType: "depends_on"}}, 1, nil
}

func newWikaGraphTestRouter(service *stubWikaGraphService, systemAdmin bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	r.Use(func(c *gin.Context) {
		c.Set(types.UserIDContextKey.String(), "u-test")
		c.Set(types.TenantIDContextKey.String(), uint64(80))
		ctx := context.WithValue(c.Request.Context(), types.SystemAdminContextKey, systemAdmin)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	h := &WikaGraphHandler{service: service}
	r.GET("/api/v1/wika/kb/:id/graph/entities", h.ListEntities)
	r.GET("/api/v1/wika/kb/:id/graph/entities/:entity_id", h.GetEntity)
	return r
}

func TestWikaGraphListEntitiesPassesTenantKBAndSystemAdminToService(t *testing.T) {
	service := &stubWikaGraphService{}
	r := newWikaGraphTestRouter(service, true)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/wika/kb/kb-team/graph/entities?limit=20&offset=10&type=concept", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if service.entityInput == nil ||
		service.entityInput.TenantID != 80 ||
		service.entityInput.KBID != "kb-team" ||
		service.entityInput.SystemAdmin != true ||
		service.entityInput.Limit != 20 ||
		service.entityInput.Offset != 10 ||
		service.entityInput.EntityType != "concept" {
		t.Fatalf("unexpected graph list input: %+v", service.entityInput)
	}
}

func TestWikaGraphGetEntityParsesEntityID(t *testing.T) {
	service := &stubWikaGraphService{}
	r := newWikaGraphTestRouter(service, false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/wika/kb/kb-team/graph/entities/42", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if service.detailInput == nil ||
		service.detailInput.TenantID != 80 ||
		service.detailInput.KBID != "kb-team" ||
		service.detailInput.EntityID != 42 {
		t.Fatalf("unexpected graph detail input: %+v", service.detailInput)
	}
}
