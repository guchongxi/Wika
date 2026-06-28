package handler

import (
	"context"
	stderrors "errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	wikasuggestion "github.com/Tencent/WeKnora/internal/wika/suggestion"
)

type wikaSuggestionService interface {
	CreateSuggestion(ctx context.Context, input wikasuggestion.CreateInput) (*wikasuggestion.SuggestionResult, error)
}

// WikaSuggestionHandler 暴露个人知识推荐到团队的入口。
type WikaSuggestionHandler struct {
	service wikaSuggestionService
}

// NewWikaSuggestionHandler 创建 Wika suggestion handler。
func NewWikaSuggestionHandler(service *wikasuggestion.Service) *WikaSuggestionHandler {
	return &WikaSuggestionHandler{service: service}
}

type createWikaSuggestionRequest struct {
	KnowledgeID    string `json:"knowledge_id"`
	TargetSpaceID  uint64 `json:"target_space_id"`
	TargetKBID     string `json:"target_kb_id"`
	Reason         string `json:"reason"`
	IdempotencyKey string `json:"idempotency_key"`
}

// CreateSuggestion 创建团队推荐并返回 AI 预审结果。
func (h *WikaSuggestionHandler) CreateSuggestion(c *gin.Context) {
	userID, tenantID, ok := wikaKnowledgeContext(c)
	if !ok {
		return
	}
	if h.service == nil {
		c.Error(apperrors.NewInternalServerError("wika suggestion service unavailable"))
		return
	}

	var req createWikaSuggestionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewBadRequestError("invalid request body").WithDetails(err.Error()))
		return
	}
	knowledgeID := strings.TrimSpace(req.KnowledgeID)
	if knowledgeID == "" {
		c.Error(apperrors.NewBadRequestError("knowledge_id is required"))
		return
	}
	targetTenantID := req.TargetSpaceID
	if targetTenantID == 0 {
		targetTenantID = tenantID
	}

	result, err := h.service.CreateSuggestion(c.Request.Context(), wikasuggestion.CreateInput{
		SubmitterID:    userID,
		KnowledgeID:    knowledgeID,
		TargetTenantID: targetTenantID,
		TargetKBID:     strings.TrimSpace(req.TargetKBID),
		Reason:         strings.TrimSpace(req.Reason),
		IdempotencyKey: strings.TrimSpace(req.IdempotencyKey),
	})
	if err != nil {
		switch {
		case stderrors.Is(err, wikasuggestion.ErrSourceKnowledgeNotPersonal):
			c.Error(apperrors.NewBadRequestError("source knowledge must belong to personal space"))
		case stderrors.Is(err, wikasuggestion.ErrSuggestionPermissionDenied):
			c.Error(apperrors.NewForbiddenError("suggestion permission denied"))
		default:
			logger.Error(c.Request.Context(), "failed to create Wika suggestion", err)
			c.Error(apperrors.NewInternalServerError("failed to create suggestion"))
		}
		return
	}
	if result == nil {
		c.Error(apperrors.NewInternalServerError("failed to create suggestion"))
		return
	}
	c.JSON(http.StatusOK, result)
}
