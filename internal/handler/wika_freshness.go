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
	wikafreshness "github.com/Tencent/WeKnora/internal/wika/freshness"
)

type wikaFreshnessService interface {
	RunCheck(ctx context.Context, input wikafreshness.RunCheckInput) (*types.WikaFreshnessCheck, error)
	ListChecks(ctx context.Context, input wikafreshness.ListInput) ([]*types.WikaFreshnessCheck, error)
	ListItems(ctx context.Context, input wikafreshness.ListInput) ([]*types.WikaFreshnessCheckItem, error)
	Overview(ctx context.Context, input wikafreshness.ListInput) (*wikafreshness.OverviewResult, error)
	HandleItem(ctx context.Context, input wikafreshness.HandleItemInput) (*types.WikaFreshnessCheckItem, error)
}

// WikaFreshnessHandler 暴露 Wika 知识保鲜扫描入口。
type WikaFreshnessHandler struct {
	service wikaFreshnessService
}

func NewWikaFreshnessHandler(service *wikafreshness.Service) *WikaFreshnessHandler {
	return &WikaFreshnessHandler{service: service}
}

type runWikaFreshnessCheckRequest struct {
	Trigger string `json:"trigger"`
}

type handleWikaFreshnessItemRequest struct {
	Action    string     `json:"action"`
	Note      string     `json:"note"`
	ExpiresAt *time.Time `json:"expires_at"`
}

// RunCheck 手动触发一次团队 KB 保鲜扫描。
func (h *WikaFreshnessHandler) RunCheck(c *gin.Context) {
	_, tenantID, ok := wikaKnowledgeContext(c)
	if !ok {
		return
	}
	if h.service == nil {
		c.Error(apperrors.NewInternalServerError("wika freshness service unavailable"))
		return
	}
	var req runWikaFreshnessCheckRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewBadRequestError("invalid request body").WithDetails(err.Error()))
		return
	}
	trigger := strings.TrimSpace(req.Trigger)
	if trigger == "" {
		trigger = wikafreshness.CheckTriggerManual
	}
	result, err := h.service.RunCheck(c.Request.Context(), wikafreshness.RunCheckInput{
		TenantID: tenantID,
		KBID:     strings.TrimSpace(c.Param("id")),
		Trigger:  trigger,
		Now:      time.Now(),
	})
	if err != nil {
		c.Error(apperrors.NewInternalServerError("failed to run freshness check"))
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *WikaFreshnessHandler) ListChecks(c *gin.Context) {
	_, tenantID, ok := wikaKnowledgeContext(c)
	if !ok {
		return
	}
	if h.service == nil {
		c.Error(apperrors.NewInternalServerError("wika freshness service unavailable"))
		return
	}
	result, err := h.service.ListChecks(c.Request.Context(), wikafreshness.ListInput{
		TenantID: tenantID,
		KBID:     strings.TrimSpace(c.Param("id")),
	})
	if err != nil {
		c.Error(apperrors.NewInternalServerError("failed to list freshness checks"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"checks": result})
}

func (h *WikaFreshnessHandler) ListItems(c *gin.Context) {
	_, tenantID, ok := wikaKnowledgeContext(c)
	if !ok {
		return
	}
	if h.service == nil {
		c.Error(apperrors.NewInternalServerError("wika freshness service unavailable"))
		return
	}
	result, err := h.service.ListItems(c.Request.Context(), wikafreshness.ListInput{
		TenantID: tenantID,
		KBID:     strings.TrimSpace(c.Param("id")),
		Status:   strings.TrimSpace(c.Query("status")),
	})
	if err != nil {
		c.Error(apperrors.NewInternalServerError("failed to list freshness items"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": result})
}

func (h *WikaFreshnessHandler) Overview(c *gin.Context) {
	_, tenantID, ok := wikaKnowledgeContext(c)
	if !ok {
		return
	}
	if h.service == nil {
		c.Error(apperrors.NewInternalServerError("wika freshness service unavailable"))
		return
	}
	result, err := h.service.Overview(c.Request.Context(), wikafreshness.ListInput{
		TenantID: tenantID,
		KBID:     strings.TrimSpace(c.Param("id")),
	})
	if err != nil {
		c.Error(apperrors.NewInternalServerError("failed to load freshness overview"))
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *WikaFreshnessHandler) HandleItem(c *gin.Context) {
	userID, tenantID, ok := wikaKnowledgeContext(c)
	if !ok {
		return
	}
	if h.service == nil {
		c.Error(apperrors.NewInternalServerError("wika freshness service unavailable"))
		return
	}
	itemID, err := strconv.ParseUint(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || itemID == 0 {
		c.Error(apperrors.NewBadRequestError("invalid freshness item id"))
		return
	}
	var req handleWikaFreshnessItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewBadRequestError("invalid request body").WithDetails(err.Error()))
		return
	}
	result, err := h.service.HandleItem(c.Request.Context(), wikafreshness.HandleItemInput{
		ActorID:   userID,
		TenantID:  tenantID,
		ItemID:    itemID,
		Action:    strings.TrimSpace(req.Action),
		Note:      strings.TrimSpace(req.Note),
		ExpiresAt: req.ExpiresAt,
		Now:       time.Now(),
	})
	if err != nil {
		switch {
		case stderrors.Is(err, wikafreshness.ErrInvalidFreshnessAction):
			c.Error(apperrors.NewBadRequestError("invalid freshness action"))
		case stderrors.Is(err, wikafreshness.ErrFreshnessItemNotFound):
			c.Error(apperrors.NewNotFoundError("freshness item not found"))
		default:
			c.Error(apperrors.NewInternalServerError("failed to handle freshness item"))
		}
		return
	}
	c.JSON(http.StatusOK, result)
}
