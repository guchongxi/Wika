package handler

import (
	"context"
	stderrors "errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
	wikaorgshare "github.com/Tencent/WeKnora/internal/wika/governance/orgshare"
)

type wikaOrgShareService interface {
	CreateShare(ctx context.Context, input wikaorgshare.CreateShareInput) (*types.WikaOrgShare, error)
	AcceptShare(ctx context.Context, input wikaorgshare.AcceptShareInput) (*types.WikaOrgShare, error)
	RevokeShare(ctx context.Context, input wikaorgshare.RevokeShareInput) (*types.WikaOrgShare, error)
}

// WikaOrgShareHandler 暴露 P5 Organization 跨团队引用共享接口。
type WikaOrgShareHandler struct {
	service wikaOrgShareService
}

func NewWikaOrgShareHandler(service *wikaorgshare.Service) *WikaOrgShareHandler {
	return &WikaOrgShareHandler{service: service}
}

type createWikaOrgShareRequest struct {
	SourceKBID     string   `json:"source_kb_id"`
	TargetTenantID uint64   `json:"target_tenant_id"`
	AllowedFields  []string `json:"allowed_fields"`
}

func (h *WikaOrgShareHandler) CreateShare(c *gin.Context) {
	userID, tenantID, ok := wikaKnowledgeContext(c)
	if !ok {
		return
	}
	if h.service == nil {
		c.Error(apperrors.NewInternalServerError("wika org share service unavailable"))
		return
	}
	var req createWikaOrgShareRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewBadRequestError("invalid request body").WithDetails(err.Error()))
		return
	}
	if strings.TrimSpace(req.SourceKBID) == "" {
		c.Error(apperrors.NewBadRequestError("source_kb_id is required"))
		return
	}
	if req.TargetTenantID == 0 {
		c.Error(apperrors.NewBadRequestError("target_tenant_id is required"))
		return
	}
	share, err := h.service.CreateShare(c.Request.Context(), wikaorgshare.CreateShareInput{
		ActorID:        userID,
		OrgID:          strings.TrimSpace(c.Param("org_id")),
		SourceTenantID: tenantID,
		SourceKBID:     strings.TrimSpace(req.SourceKBID),
		TargetTenantID: req.TargetTenantID,
		AllowedFields:  req.AllowedFields,
		Now:            time.Now(),
	})
	if err != nil {
		h.handleOrgShareError(c, err, "failed to create org share")
		return
	}
	c.JSON(http.StatusOK, share)
}

func (h *WikaOrgShareHandler) AcceptShare(c *gin.Context) {
	userID, _, ok := wikaKnowledgeContext(c)
	if !ok {
		return
	}
	if h.service == nil {
		c.Error(apperrors.NewInternalServerError("wika org share service unavailable"))
		return
	}
	shareID, ok := parseOrgShareID(c)
	if !ok {
		return
	}
	share, err := h.service.AcceptShare(c.Request.Context(), wikaorgshare.AcceptShareInput{
		ActorID: userID,
		ShareID: shareID,
		Now:     time.Now(),
	})
	if err != nil {
		h.handleOrgShareError(c, err, "failed to accept org share")
		return
	}
	c.JSON(http.StatusOK, share)
}

func (h *WikaOrgShareHandler) RevokeShare(c *gin.Context) {
	userID, _, ok := wikaKnowledgeContext(c)
	if !ok {
		return
	}
	if h.service == nil {
		c.Error(apperrors.NewInternalServerError("wika org share service unavailable"))
		return
	}
	shareID, ok := parseOrgShareID(c)
	if !ok {
		return
	}
	share, err := h.service.RevokeShare(c.Request.Context(), wikaorgshare.RevokeShareInput{
		ActorID: userID,
		ShareID: shareID,
		Now:     time.Now(),
	})
	if err != nil {
		h.handleOrgShareError(c, err, "failed to revoke org share")
		return
	}
	c.JSON(http.StatusOK, share)
}

func parseOrgShareID(c *gin.Context) (uint64, bool) {
	shareID, err := strconv.ParseUint(strings.TrimSpace(c.Param("share_id")), 10, 64)
	if err != nil || shareID == 0 {
		c.Error(apperrors.NewBadRequestError("invalid org share id"))
		return 0, false
	}
	return shareID, true
}

func (h *WikaOrgShareHandler) handleOrgShareError(c *gin.Context, err error, fallback string) {
	switch {
	case stderrors.Is(err, wikaorgshare.ErrFeatureDisabled):
		c.Error(apperrors.NewNotFoundError("wika org share feature disabled"))
	case stderrors.Is(err, wikaorgshare.ErrInvalidAllowedFields):
		c.Error(apperrors.NewBadRequestError("invalid allowed_fields"))
	case stderrors.Is(err, wikaorgshare.ErrScopeDenied):
		c.Error(apperrors.NewForbiddenError("org share scope denied"))
	case stderrors.Is(err, wikaorgshare.ErrShareNotFound):
		c.Error(apperrors.NewNotFoundError("org share not found"))
	case stderrors.Is(err, wikaorgshare.ErrInvalidShareState):
		c.Error(apperrors.NewConflictError("org share state conflict"))
	default:
		c.Error(apperrors.NewInternalServerError(fallback))
	}
}
