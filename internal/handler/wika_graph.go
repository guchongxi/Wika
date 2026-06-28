package handler

import (
	"context"
	stderrors "errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
	wikagraph "github.com/Tencent/WeKnora/internal/wika/graph"
)

type wikaGraphService interface {
	Overview(ctx context.Context, input wikagraph.OverviewInput) (*wikagraph.Overview, error)
	ListEntities(ctx context.Context, input wikagraph.ListEntitiesInput) ([]*types.WikaGraphEntity, int64, error)
	GetEntity(ctx context.Context, input wikagraph.GetEntityInput) (*types.WikaGraphEntity, error)
	ListEdges(ctx context.Context, input wikagraph.ListEdgesInput) ([]*types.WikaGraphEdge, int64, error)
}

// WikaGraphHandler 暴露 Wika 图谱读模型接口。
type WikaGraphHandler struct {
	service wikaGraphService
}

func NewWikaGraphHandler(service *wikagraph.Service) *WikaGraphHandler {
	return &WikaGraphHandler{service: service}
}

func (h *WikaGraphHandler) Overview(c *gin.Context) {
	_, tenantID, ok := wikaKnowledgeContext(c)
	if !ok {
		return
	}
	if h.service == nil {
		c.Error(apperrors.NewInternalServerError("wika graph service unavailable"))
		return
	}
	result, err := h.service.Overview(c.Request.Context(), wikagraph.OverviewInput{
		TenantID:    tenantID,
		KBID:        strings.TrimSpace(c.Param("id")),
		SystemAdmin: types.IsSystemAdminFromContext(c.Request.Context()),
	})
	if err != nil {
		c.Error(apperrors.NewInternalServerError("failed to load graph overview"))
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *WikaGraphHandler) ListEntities(c *gin.Context) {
	_, tenantID, ok := wikaKnowledgeContext(c)
	if !ok {
		return
	}
	if h.service == nil {
		c.Error(apperrors.NewInternalServerError("wika graph service unavailable"))
		return
	}
	entities, total, err := h.service.ListEntities(c.Request.Context(), wikagraph.ListEntitiesInput{
		TenantID:    tenantID,
		KBID:        strings.TrimSpace(c.Param("id")),
		EntityType:  strings.TrimSpace(c.Query("type")),
		Query:       strings.TrimSpace(c.Query("q")),
		Limit:       parseWikaGraphInt(c.Query("limit"), 50),
		Offset:      parseWikaGraphInt(c.Query("offset"), 0),
		SystemAdmin: types.IsSystemAdminFromContext(c.Request.Context()),
	})
	if err != nil {
		c.Error(apperrors.NewInternalServerError("failed to list graph entities"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"entities": entities, "total": total})
}

func (h *WikaGraphHandler) GetEntity(c *gin.Context) {
	_, tenantID, ok := wikaKnowledgeContext(c)
	if !ok {
		return
	}
	if h.service == nil {
		c.Error(apperrors.NewInternalServerError("wika graph service unavailable"))
		return
	}
	entityID, err := strconv.ParseUint(strings.TrimSpace(c.Param("entity_id")), 10, 64)
	if err != nil || entityID == 0 {
		c.Error(apperrors.NewBadRequestError("invalid graph entity id"))
		return
	}
	entity, err := h.service.GetEntity(c.Request.Context(), wikagraph.GetEntityInput{
		TenantID:    tenantID,
		KBID:        strings.TrimSpace(c.Param("id")),
		EntityID:    entityID,
		SystemAdmin: types.IsSystemAdminFromContext(c.Request.Context()),
	})
	if err != nil {
		if stderrors.Is(err, wikagraph.ErrGraphEntityNotFound) {
			c.Error(apperrors.NewNotFoundError("graph entity not found"))
			return
		}
		c.Error(apperrors.NewInternalServerError("failed to load graph entity"))
		return
	}
	c.JSON(http.StatusOK, entity)
}

func (h *WikaGraphHandler) ListEdges(c *gin.Context) {
	_, tenantID, ok := wikaKnowledgeContext(c)
	if !ok {
		return
	}
	if h.service == nil {
		c.Error(apperrors.NewInternalServerError("wika graph service unavailable"))
		return
	}
	edges, total, err := h.service.ListEdges(c.Request.Context(), wikagraph.ListEdgesInput{
		TenantID:          tenantID,
		KBID:              strings.TrimSpace(c.Param("id")),
		SourceEntityID:    uint64(parseWikaGraphInt(c.Query("source_entity_id"), 0)),
		TargetEntityID:    uint64(parseWikaGraphInt(c.Query("target_entity_id"), 0)),
		EvidenceKnowledge: strings.TrimSpace(c.Query("evidence_knowledge_id")),
		Limit:             parseWikaGraphInt(c.Query("limit"), 50),
		Offset:            parseWikaGraphInt(c.Query("offset"), 0),
		SystemAdmin:       types.IsSystemAdminFromContext(c.Request.Context()),
	})
	if err != nil {
		c.Error(apperrors.NewInternalServerError("failed to list graph edges"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"edges": edges, "total": total})
}

func parseWikaGraphInt(raw string, fallback int) int {
	if strings.TrimSpace(raw) == "" {
		return fallback
	}
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return fallback
	}
	return value
}
