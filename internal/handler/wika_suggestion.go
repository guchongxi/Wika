package handler

import (
	"context"
	stderrors "errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	wikasuggestion "github.com/Tencent/WeKnora/internal/wika/suggestion"
)

type wikaSuggestionService interface {
	CreateSuggestion(ctx context.Context, input wikasuggestion.CreateInput) (*wikasuggestion.SuggestionResult, error)
	HumanReview(ctx context.Context, input wikasuggestion.HumanReviewInput) (*wikasuggestion.SuggestionResult, error)
	ApplySuggestion(ctx context.Context, input wikasuggestion.ApplyInput) (*wikasuggestion.ApplyResult, error)
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

type humanReviewWikaSuggestionRequest struct {
	FinalDecision string   `json:"final_decision"`
	Title         string   `json:"title"`
	Content       string   `json:"content"`
	Tags          []string `json:"tags"`
	TargetKBID    string   `json:"target_kb_id"`
	Comment       string   `json:"comment"`
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

// HumanReview 保存团队维护者的人工审核和修正结果。
func (h *WikaSuggestionHandler) HumanReview(c *gin.Context) {
	userID, _, ok := wikaKnowledgeContext(c)
	if !ok {
		return
	}
	if h.service == nil {
		c.Error(apperrors.NewInternalServerError("wika suggestion service unavailable"))
		return
	}
	suggestionID, ok := parseWikaSuggestionID(c)
	if !ok {
		return
	}

	var req humanReviewWikaSuggestionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewBadRequestError("invalid request body").WithDetails(err.Error()))
		return
	}
	decision := wikasuggestion.Decision(strings.TrimSpace(req.FinalDecision))
	if decision == "" {
		c.Error(apperrors.NewBadRequestError("final_decision is required"))
		return
	}

	result, err := h.service.HumanReview(c.Request.Context(), wikasuggestion.HumanReviewInput{
		ActorID:       userID,
		SuggestionID:  suggestionID,
		FinalDecision: decision,
		Title:         strings.TrimSpace(req.Title),
		Content:       strings.TrimSpace(req.Content),
		Tags:          req.Tags,
		TargetKBID:    strings.TrimSpace(req.TargetKBID),
		Comment:       strings.TrimSpace(req.Comment),
	})
	if err != nil {
		h.handleSuggestionError(c, err, "failed to review suggestion")
		return
	}
	c.JSON(http.StatusOK, result)
}

// ApplySuggestion 将已通过的推荐复制到目标团队知识库。
func (h *WikaSuggestionHandler) ApplySuggestion(c *gin.Context) {
	userID, _, ok := wikaKnowledgeContext(c)
	if !ok {
		return
	}
	if h.service == nil {
		c.Error(apperrors.NewInternalServerError("wika suggestion service unavailable"))
		return
	}
	suggestionID, ok := parseWikaSuggestionID(c)
	if !ok {
		return
	}

	result, err := h.service.ApplySuggestion(c.Request.Context(), wikasuggestion.ApplyInput{
		ActorID:      userID,
		SuggestionID: suggestionID,
	})
	if err != nil {
		h.handleSuggestionError(c, err, "failed to apply suggestion")
		return
	}
	c.JSON(http.StatusOK, result)
}

func parseWikaSuggestionID(c *gin.Context) (uint64, bool) {
	id, err := strconv.ParseUint(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id == 0 {
		c.Error(apperrors.NewBadRequestError("invalid suggestion id"))
		return 0, false
	}
	return id, true
}

func (h *WikaSuggestionHandler) handleSuggestionError(c *gin.Context, err error, fallback string) {
	switch {
	case stderrors.Is(err, wikasuggestion.ErrSourceKnowledgeNotPersonal):
		c.Error(apperrors.NewBadRequestError("source knowledge must belong to personal space"))
	case stderrors.Is(err, wikasuggestion.ErrInvalidDecision):
		c.Error(apperrors.NewBadRequestError("invalid final_decision"))
	case stderrors.Is(err, wikasuggestion.ErrSuggestionPermissionDenied):
		c.Error(apperrors.NewForbiddenError("suggestion permission denied"))
	case stderrors.Is(err, wikasuggestion.ErrSuggestionNotFound):
		c.Error(apperrors.NewNotFoundError("suggestion not found"))
	case stderrors.Is(err, wikasuggestion.ErrSuggestionNotApproved):
		c.Error(apperrors.NewBadRequestError("suggestion is not approved"))
	case stderrors.Is(err, wikasuggestion.ErrSourceKnowledgeChanged):
		c.Error(apperrors.NewConflictError("source knowledge changed since review"))
	default:
		logger.Error(c.Request.Context(), fallback, err)
		c.Error(apperrors.NewInternalServerError(fallback))
	}
}
