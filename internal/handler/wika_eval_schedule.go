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
	List(ctx context.Context, input wikaevalschedule.ListInput) (*wikaevalschedule.ListResult, error)
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

func (h *WikaEvalScheduleHandler) List(c *gin.Context) {
	userID, tenantID, ok := wikaKnowledgeContext(c)
	if !ok {
		return
	}
	if h.service == nil {
		c.Error(apperrors.NewInternalServerError("wika eval schedule service unavailable"))
		return
	}
	enabled, okEnabled := parseEvalScheduleBoolQuery(c.Query("enabled"))
	limit := parseEvalScheduleIntQuery(c.Query("limit"), 50)
	offset := parseEvalScheduleIntQuery(c.Query("offset"), 0)
	input := wikaevalschedule.ListInput{
		ActorID:  userID,
		TenantID: tenantID,
		KBID:     strings.TrimSpace(c.Param("id")),
		Limit:    limit,
		Offset:   offset,
	}
	if okEnabled {
		input.Enabled = &enabled
	}
	result, err := h.service.List(c.Request.Context(), input)
	if err != nil {
		h.handleScheduleError(c, err, "failed to list eval schedules")
		return
	}
	c.JSON(http.StatusOK, result)
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

func parseEvalScheduleBoolQuery(raw string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "true", "1":
		return true, true
	case "false", "0":
		return false, true
	default:
		return false, false
	}
}

func parseEvalScheduleIntQuery(raw string, fallback int) int {
	if strings.TrimSpace(raw) == "" {
		return fallback
	}
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return fallback
	}
	return v
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
	case stderrors.Is(err, wikaevalschedule.ErrScheduleConflict):
		c.Error(apperrors.NewConflictError("enabled eval schedule already exists for dataset"))
	default:
		c.Error(apperrors.NewInternalServerError(fallback))
	}
}
