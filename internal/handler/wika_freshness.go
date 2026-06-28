package handler

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
	wikafreshness "github.com/Tencent/WeKnora/internal/wika/freshness"
)

type wikaFreshnessService interface {
	RunCheck(ctx context.Context, input wikafreshness.RunCheckInput) (*types.WikaFreshnessCheck, error)
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
