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
	wikaevalschedule "github.com/Tencent/WeKnora/internal/wika/governance/evalschedule"
)

type wikaEvalScheduleService interface {
	CreateSchedule(ctx context.Context, input wikaevalschedule.CreateScheduleInput) (*types.WikaEvalSchedule, error)
	UpdateSchedule(ctx context.Context, input wikaevalschedule.UpdateScheduleInput) (*types.WikaEvalSchedule, error)
	DisableSchedule(ctx context.Context, input wikaevalschedule.DisableScheduleInput) (*types.WikaEvalSchedule, error)
}

// WikaEvalScheduleHandler 暴露 P5 定时评测计划接口。
type WikaEvalScheduleHandler struct {
	service wikaEvalScheduleService
}

func NewWikaEvalScheduleHandler(service *wikaevalschedule.Service) *WikaEvalScheduleHandler {
	return &WikaEvalScheduleHandler{service: service}
}

type createWikaEvalScheduleRequest struct {
	DatasetID uint64 `json:"dataset_id"`
	CronExpr  string `json:"cron_expr"`
	Enabled   bool   `json:"enabled"`
}

type updateWikaEvalScheduleRequest struct {
	CronExpr string `json:"cron_expr"`
	Enabled  bool   `json:"enabled"`
}

func (h *WikaEvalScheduleHandler) CreateSchedule(c *gin.Context) {
	userID, tenantID, ok := wikaKnowledgeContext(c)
	if !ok {
		return
	}
	if h.service == nil {
		c.Error(apperrors.NewInternalServerError("wika eval schedule service unavailable"))
		return
	}
	var req createWikaEvalScheduleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewBadRequestError("invalid request body").WithDetails(err.Error()))
		return
	}
	if req.DatasetID == 0 {
		c.Error(apperrors.NewBadRequestError("dataset_id is required"))
		return
	}
	result, err := h.service.CreateSchedule(c.Request.Context(), wikaevalschedule.CreateScheduleInput{
		ActorID:   userID,
		TenantID:  tenantID,
		KBID:      strings.TrimSpace(c.Param("id")),
		DatasetID: req.DatasetID,
		CronExpr:  strings.TrimSpace(req.CronExpr),
		Enabled:   req.Enabled,
		Now:       time.Now(),
	})
	if err != nil {
		h.handleScheduleError(c, err, "failed to create eval schedule")
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *WikaEvalScheduleHandler) UpdateSchedule(c *gin.Context) {
	userID, _, ok := wikaKnowledgeContext(c)
	if !ok {
		return
	}
	if h.service == nil {
		c.Error(apperrors.NewInternalServerError("wika eval schedule service unavailable"))
		return
	}
	scheduleID, err := strconv.ParseUint(strings.TrimSpace(c.Param("schedule_id")), 10, 64)
	if err != nil || scheduleID == 0 {
		c.Error(apperrors.NewBadRequestError("invalid eval schedule id"))
		return
	}
	var req updateWikaEvalScheduleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewBadRequestError("invalid request body").WithDetails(err.Error()))
		return
	}
	result, err := h.service.UpdateSchedule(c.Request.Context(), wikaevalschedule.UpdateScheduleInput{
		ActorID:    userID,
		ScheduleID: scheduleID,
		CronExpr:   strings.TrimSpace(req.CronExpr),
		Enabled:    req.Enabled,
		Now:        time.Now(),
	})
	if err != nil {
		h.handleScheduleError(c, err, "failed to update eval schedule")
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *WikaEvalScheduleHandler) DisableSchedule(c *gin.Context) {
	userID, _, ok := wikaKnowledgeContext(c)
	if !ok {
		return
	}
	if h.service == nil {
		c.Error(apperrors.NewInternalServerError("wika eval schedule service unavailable"))
		return
	}
	scheduleID, err := strconv.ParseUint(strings.TrimSpace(c.Param("schedule_id")), 10, 64)
	if err != nil || scheduleID == 0 {
		c.Error(apperrors.NewBadRequestError("invalid eval schedule id"))
		return
	}
	result, err := h.service.DisableSchedule(c.Request.Context(), wikaevalschedule.DisableScheduleInput{
		ActorID:    userID,
		ScheduleID: scheduleID,
		Now:        time.Now(),
	})
	if err != nil {
		h.handleScheduleError(c, err, "failed to disable eval schedule")
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *WikaEvalScheduleHandler) handleScheduleError(c *gin.Context, err error, fallback string) {
	switch {
	case stderrors.Is(err, wikaevalschedule.ErrFeatureDisabled):
		c.Error(apperrors.NewNotFoundError("wika eval schedule feature disabled"))
	case stderrors.Is(err, wikaevalschedule.ErrInvalidSchedule):
		c.Error(apperrors.NewBadRequestError("invalid eval schedule"))
	case stderrors.Is(err, wikaevalschedule.ErrScheduleNotFound):
		c.Error(apperrors.NewNotFoundError("eval schedule not found"))
	default:
		c.Error(apperrors.NewInternalServerError(fallback))
	}
}
