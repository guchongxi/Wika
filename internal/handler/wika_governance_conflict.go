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
	wikaconflict "github.com/Tencent/WeKnora/internal/wika/governance/conflict"
)

type wikaConflictService interface {
	CreateCheck(ctx context.Context, input wikaconflict.CreateCheckInput) (*types.WikaConflictCheck, error)
	ListItems(ctx context.Context, input wikaconflict.ListItemsInput) ([]*types.WikaConflictItem, int64, error)
	ResolveItem(ctx context.Context, input wikaconflict.ResolveItemInput) (*types.WikaConflictItem, error)
}

// WikaConflictHandler 暴露 P5 冲突治理接口。
type WikaConflictHandler struct {
	service wikaConflictService
}

func NewWikaConflictHandler(service *wikaconflict.Service) *WikaConflictHandler {
	return &WikaConflictHandler{service: service}
}

type createWikaConflictCheckRequest struct {
	Trigger string `json:"trigger"`
}

type resolveWikaConflictItemRequest struct {
	Status  string `json:"status"`
	Comment string `json:"comment"`
}

func (h *WikaConflictHandler) CreateCheck(c *gin.Context) {
	userID, tenantID, ok := wikaKnowledgeContext(c)
	if !ok {
		return
	}
	if h.service == nil {
		c.Error(apperrors.NewInternalServerError("wika conflict service unavailable"))
		return
	}
	var req createWikaConflictCheckRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewBadRequestError("invalid request body").WithDetails(err.Error()))
		return
	}
	trigger := strings.TrimSpace(req.Trigger)
	if trigger == "" {
		trigger = wikaconflict.TriggerManual
	}
	check, err := h.service.CreateCheck(c.Request.Context(), wikaconflict.CreateCheckInput{
		ActorID:  userID,
		TenantID: tenantID,
		KBID:     strings.TrimSpace(c.Param("id")),
		Trigger:  trigger,
		Now:      time.Now(),
	})
	if err != nil {
		if stderrors.Is(err, wikaconflict.ErrFeatureDisabled) {
			c.Error(apperrors.NewNotFoundError("wika conflict feature disabled"))
			return
		}
		c.Error(apperrors.NewInternalServerError("failed to create conflict check"))
		return
	}
	c.JSON(http.StatusOK, check)
}

func (h *WikaConflictHandler) ListItems(c *gin.Context) {
	userID, tenantID, ok := wikaKnowledgeContext(c)
	if !ok {
		return
	}
	if h.service == nil {
		c.Error(apperrors.NewInternalServerError("wika conflict service unavailable"))
		return
	}
	items, total, err := h.service.ListItems(c.Request.Context(), wikaconflict.ListItemsInput{
		ActorID:  userID,
		TenantID: tenantID,
		KBID:     strings.TrimSpace(c.Param("id")),
		Status:   strings.TrimSpace(c.Query("status")),
		Limit:    parseWikaConflictInt(c.Query("limit"), 20),
		Offset:   parseWikaConflictInt(c.Query("offset"), 0),
	})
	if err != nil {
		if stderrors.Is(err, wikaconflict.ErrFeatureDisabled) {
			c.Error(apperrors.NewNotFoundError("wika conflict feature disabled"))
			return
		}
		c.Error(apperrors.NewInternalServerError("failed to list conflict items"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": total})
}

func (h *WikaConflictHandler) ResolveItem(c *gin.Context) {
	userID, tenantID, ok := wikaKnowledgeContext(c)
	if !ok {
		return
	}
	if h.service == nil {
		c.Error(apperrors.NewInternalServerError("wika conflict service unavailable"))
		return
	}
	itemID, err := strconv.ParseUint(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || itemID == 0 {
		c.Error(apperrors.NewBadRequestError("invalid conflict item id"))
		return
	}
	var req resolveWikaConflictItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewBadRequestError("invalid request body").WithDetails(err.Error()))
		return
	}
	item, err := h.service.ResolveItem(c.Request.Context(), wikaconflict.ResolveItemInput{
		ActorID:  userID,
		TenantID: tenantID,
		ItemID:   itemID,
		Status:   strings.TrimSpace(req.Status),
		Comment:  strings.TrimSpace(req.Comment),
		Now:      time.Now(),
	})
	if err != nil {
		switch {
		case stderrors.Is(err, wikaconflict.ErrFeatureDisabled):
			c.Error(apperrors.NewNotFoundError("wika conflict feature disabled"))
		case stderrors.Is(err, wikaconflict.ErrInvalidConflictStatus):
			c.Error(apperrors.NewBadRequestError("invalid conflict status"))
		case stderrors.Is(err, wikaconflict.ErrConflictItemNotFound):
			c.Error(apperrors.NewNotFoundError("conflict item not found"))
		case stderrors.Is(err, wikaconflict.ErrConflictItemTerminal):
			c.Error(apperrors.NewConflictError("conflict item is terminal"))
		default:
			c.Error(apperrors.NewInternalServerError("failed to resolve conflict item"))
		}
		return
	}
	c.JSON(http.StatusOK, item)
}

func parseWikaConflictInt(raw string, fallback int) int {
	if strings.TrimSpace(raw) == "" {
		return fallback
	}
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return fallback
	}
	return value
}
