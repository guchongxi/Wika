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
	wikaversion "github.com/Tencent/WeKnora/internal/wika/governance/version"
)

type wikaVersionService interface {
	ListVersions(ctx context.Context, input wikaversion.ListVersionsInput) ([]*types.WikaKnowledgeVersion, error)
	Diff(ctx context.Context, input wikaversion.DiffInput) (*wikaversion.DiffResult, error)
}

// WikaVersionHandler 暴露 P5 知识版本列表和 diff 接口。
type WikaVersionHandler struct {
	service wikaVersionService
}

func NewWikaVersionHandler(service *wikaversion.Service) *WikaVersionHandler {
	return &WikaVersionHandler{service: service}
}

func (h *WikaVersionHandler) ListVersions(c *gin.Context) {
	userID, tenantID, ok := wikaKnowledgeContext(c)
	if !ok {
		return
	}
	if h.service == nil {
		c.Error(apperrors.NewInternalServerError("wika version service unavailable"))
		return
	}
	versions, err := h.service.ListVersions(c.Request.Context(), wikaversion.ListVersionsInput{
		ActorID:     userID,
		TenantID:    tenantID,
		KnowledgeID: strings.TrimSpace(c.Param("id")),
		Limit:       parseWikaVersionInt(c.Query("limit"), 50),
		Offset:      parseWikaVersionInt(c.Query("offset"), 0),
		SystemAdmin: types.IsSystemAdminFromContext(c.Request.Context()),
	})
	if err != nil {
		c.Error(apperrors.NewInternalServerError("failed to list knowledge versions"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"versions": versions})
}

func (h *WikaVersionHandler) Diff(c *gin.Context) {
	userID, tenantID, ok := wikaKnowledgeContext(c)
	if !ok {
		return
	}
	if h.service == nil {
		c.Error(apperrors.NewInternalServerError("wika version service unavailable"))
		return
	}
	fromVersion, err := strconv.ParseUint(strings.TrimSpace(c.Param("version_id")), 10, 64)
	if err != nil || fromVersion == 0 {
		c.Error(apperrors.NewBadRequestError("invalid from version id"))
		return
	}
	toVersion, err := strconv.ParseUint(strings.TrimSpace(c.Query("to_version_id")), 10, 64)
	if err != nil || toVersion == 0 {
		c.Error(apperrors.NewBadRequestError("invalid to version id"))
		return
	}
	diff, err := h.service.Diff(c.Request.Context(), wikaversion.DiffInput{
		ActorID:     userID,
		TenantID:    tenantID,
		KnowledgeID: strings.TrimSpace(c.Param("id")),
		FromVersion: fromVersion,
		ToVersion:   toVersion,
		SystemAdmin: types.IsSystemAdminFromContext(c.Request.Context()),
	})
	if err != nil {
		if stderrors.Is(err, wikaversion.ErrVersionNotFound) {
			c.Error(apperrors.NewNotFoundError("knowledge version not found"))
			return
		}
		c.Error(apperrors.NewInternalServerError("failed to diff knowledge versions"))
		return
	}
	c.JSON(http.StatusOK, diff)
}

func parseWikaVersionInt(raw string, fallback int) int {
	if strings.TrimSpace(raw) == "" {
		return fallback
	}
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return fallback
	}
	return value
}
