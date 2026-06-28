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
	wikaurlrefresh "github.com/Tencent/WeKnora/internal/wika/governance/urlrefresh"
)

type wikaURLRefreshService interface {
	CreateJob(ctx context.Context, input wikaurlrefresh.CreateJobInput) (*types.WikaURLRefreshJob, error)
	CreateOrUpdateSchedule(ctx context.Context, input wikaurlrefresh.CreateOrUpdateScheduleInput) (*types.WikaURLRefreshSchedule, error)
	UpdateSchedule(ctx context.Context, input wikaurlrefresh.UpdateScheduleInput) (*types.WikaURLRefreshSchedule, error)
	DisableSchedule(ctx context.Context, input wikaurlrefresh.DisableScheduleInput) (*types.WikaURLRefreshSchedule, error)
	ReviewJob(ctx context.Context, input wikaurlrefresh.ReviewJobInput) (*wikaurlrefresh.ReviewJobResult, error)
}

// WikaURLRefreshHandler 暴露 P5 URL 重抓任务和审核接口。
type WikaURLRefreshHandler struct {
	service wikaURLRefreshService
}

func NewWikaURLRefreshHandler(service *wikaurlrefresh.Service) *WikaURLRefreshHandler {
	return &WikaURLRefreshHandler{service: service}
}

type createURLRefreshJobRequest struct {
	SourceURL string                     `json:"source_url"`
	Schedule  *urlRefreshScheduleRequest `json:"schedule"`
}

type urlRefreshScheduleRequest struct {
	Enabled  bool   `json:"enabled"`
	CronExpr string `json:"cron_expr"`
}

type reviewURLRefreshJobRequest struct {
	Decision string `json:"decision"`
	Comment  string `json:"comment"`
}

type updateURLRefreshScheduleRequest struct {
	Enabled  bool   `json:"enabled"`
	CronExpr string `json:"cron_expr"`
}

func (h *WikaURLRefreshHandler) CreateJob(c *gin.Context) {
	userID, tenantID, ok := wikaKnowledgeContext(c)
	if !ok {
		return
	}
	if h.service == nil {
		c.Error(apperrors.NewInternalServerError("wika url refresh service unavailable"))
		return
	}
	var req createURLRefreshJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewBadRequestError("invalid request body").WithDetails(err.Error()))
		return
	}
	if req.Schedule != nil {
		schedule, err := h.service.CreateOrUpdateSchedule(c.Request.Context(), wikaurlrefresh.CreateOrUpdateScheduleInput{
			ActorID:     userID,
			TenantID:    tenantID,
			KnowledgeID: strings.TrimSpace(c.Param("id")),
			SourceURL:   strings.TrimSpace(req.SourceURL),
			CronExpr:    strings.TrimSpace(req.Schedule.CronExpr),
			Enabled:     req.Schedule.Enabled,
			Now:         time.Now(),
		})
		if err != nil {
			switch {
			case stderrors.Is(err, wikaurlrefresh.ErrFeatureDisabled):
				c.Error(apperrors.NewNotFoundError("wika url refresh feature disabled"))
			case stderrors.Is(err, wikaurlrefresh.ErrInvalidSchedule):
				c.Error(apperrors.NewBadRequestError("invalid url refresh schedule"))
			default:
				c.Error(apperrors.NewInternalServerError("failed to update url refresh schedule"))
			}
			return
		}
		c.JSON(http.StatusOK, schedule)
		return
	}
	job, err := h.service.CreateJob(c.Request.Context(), wikaurlrefresh.CreateJobInput{
		ActorID:     userID,
		TenantID:    tenantID,
		KnowledgeID: strings.TrimSpace(c.Param("id")),
		SourceURL:   strings.TrimSpace(req.SourceURL),
		Now:         time.Now(),
	})
	if err != nil {
		if stderrors.Is(err, wikaurlrefresh.ErrFeatureDisabled) {
			c.Error(apperrors.NewNotFoundError("wika url refresh feature disabled"))
			return
		}
		c.Error(apperrors.NewInternalServerError("failed to create url refresh job"))
		return
	}
	c.JSON(http.StatusOK, job)
}

func (h *WikaURLRefreshHandler) UpdateSchedule(c *gin.Context) {
	userID, _, ok := wikaKnowledgeContext(c)
	if !ok {
		return
	}
	if h.service == nil {
		c.Error(apperrors.NewInternalServerError("wika url refresh service unavailable"))
		return
	}
	scheduleID, err := strconv.ParseUint(strings.TrimSpace(c.Param("schedule_id")), 10, 64)
	if err != nil || scheduleID == 0 {
		c.Error(apperrors.NewBadRequestError("invalid url refresh schedule id"))
		return
	}
	var req updateURLRefreshScheduleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewBadRequestError("invalid request body").WithDetails(err.Error()))
		return
	}
	schedule, err := h.service.UpdateSchedule(c.Request.Context(), wikaurlrefresh.UpdateScheduleInput{
		ActorID:    userID,
		ScheduleID: scheduleID,
		CronExpr:   strings.TrimSpace(req.CronExpr),
		Enabled:    req.Enabled,
		Now:        time.Now(),
	})
	if err != nil {
		h.handleScheduleError(c, err)
		return
	}
	c.JSON(http.StatusOK, schedule)
}

func (h *WikaURLRefreshHandler) DisableSchedule(c *gin.Context) {
	userID, _, ok := wikaKnowledgeContext(c)
	if !ok {
		return
	}
	if h.service == nil {
		c.Error(apperrors.NewInternalServerError("wika url refresh service unavailable"))
		return
	}
	scheduleID, err := strconv.ParseUint(strings.TrimSpace(c.Param("schedule_id")), 10, 64)
	if err != nil || scheduleID == 0 {
		c.Error(apperrors.NewBadRequestError("invalid url refresh schedule id"))
		return
	}
	schedule, err := h.service.DisableSchedule(c.Request.Context(), wikaurlrefresh.DisableScheduleInput{
		ActorID:    userID,
		ScheduleID: scheduleID,
		Now:        time.Now(),
	})
	if err != nil {
		h.handleScheduleError(c, err)
		return
	}
	c.JSON(http.StatusOK, schedule)
}

func (h *WikaURLRefreshHandler) ReviewJob(c *gin.Context) {
	userID, _, ok := wikaKnowledgeContext(c)
	if !ok {
		return
	}
	if h.service == nil {
		c.Error(apperrors.NewInternalServerError("wika url refresh service unavailable"))
		return
	}
	jobID, err := strconv.ParseUint(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || jobID == 0 {
		c.Error(apperrors.NewBadRequestError("invalid url refresh job id"))
		return
	}
	var req reviewURLRefreshJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewBadRequestError("invalid request body").WithDetails(err.Error()))
		return
	}
	result, err := h.service.ReviewJob(c.Request.Context(), wikaurlrefresh.ReviewJobInput{
		ActorID:  userID,
		JobID:    jobID,
		Decision: strings.TrimSpace(req.Decision),
		Comment:  strings.TrimSpace(req.Comment),
		Now:      time.Now(),
	})
	if err != nil {
		switch {
		case stderrors.Is(err, wikaurlrefresh.ErrFeatureDisabled):
			c.Error(apperrors.NewNotFoundError("wika url refresh feature disabled"))
		case stderrors.Is(err, wikaurlrefresh.ErrInvalidReviewDecision):
			c.Error(apperrors.NewBadRequestError("invalid review decision"))
		case stderrors.Is(err, wikaurlrefresh.ErrInvalidJobState):
			c.Error(apperrors.NewConflictError("url refresh job state conflict"))
		case stderrors.Is(err, wikaurlrefresh.ErrJobNotFound):
			c.Error(apperrors.NewNotFoundError("url refresh job not found"))
		default:
			c.Error(apperrors.NewInternalServerError("failed to review url refresh job"))
		}
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *WikaURLRefreshHandler) handleScheduleError(c *gin.Context, err error) {
	switch {
	case stderrors.Is(err, wikaurlrefresh.ErrFeatureDisabled):
		c.Error(apperrors.NewNotFoundError("wika url refresh feature disabled"))
	case stderrors.Is(err, wikaurlrefresh.ErrInvalidSchedule):
		c.Error(apperrors.NewBadRequestError("invalid url refresh schedule"))
	case stderrors.Is(err, wikaurlrefresh.ErrScheduleNotFound):
		c.Error(apperrors.NewNotFoundError("url refresh schedule not found"))
	default:
		c.Error(apperrors.NewInternalServerError("failed to update url refresh schedule"))
	}
}
