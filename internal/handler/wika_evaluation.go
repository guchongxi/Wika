package handler

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
	wikaeval "github.com/Tencent/WeKnora/internal/wika/evaluation"
)

type wikaEvaluationService interface {
	CreateDataset(ctx context.Context, input wikaeval.CreateDatasetInput) (*types.WikaEvalDataset, error)
	AddQAItem(ctx context.Context, input wikaeval.AddQAItemInput) (*types.WikaEvalQAItem, error)
	RunEvaluation(ctx context.Context, input wikaeval.RunInput) (*types.WikaEvalRun, error)
}

// WikaEvaluationHandler 暴露 Wika 质量评测数据集入口。
type WikaEvaluationHandler struct {
	service wikaEvaluationService
}

func NewWikaEvaluationHandler(service *wikaeval.Service) *WikaEvaluationHandler {
	return &WikaEvaluationHandler{service: service}
}

type createWikaEvalDatasetRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type addWikaEvalQAItemRequest struct {
	Question             string   `json:"question"`
	ExpectedAnswer       string   `json:"expected_answer"`
	ExpectedKnowledgeIDs []string `json:"expected_knowledge_ids"`
	ExpectedChunkIDs     []string `json:"expected_chunk_ids"`
	Tags                 []string `json:"tags"`
	Enabled              *bool    `json:"enabled"`
}

type runWikaEvalRequest struct {
	DatasetID uint64 `json:"dataset_id"`
}

// CreateDataset 创建团队 KB 的黄金 QA 数据集。
func (h *WikaEvaluationHandler) CreateDataset(c *gin.Context) {
	userID, tenantID, ok := wikaKnowledgeContext(c)
	if !ok {
		return
	}
	if h.service == nil {
		c.Error(apperrors.NewInternalServerError("wika evaluation service unavailable"))
		return
	}
	var req createWikaEvalDatasetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewBadRequestError("invalid request body").WithDetails(err.Error()))
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		c.Error(apperrors.NewBadRequestError("name is required"))
		return
	}
	result, err := h.service.CreateDataset(c.Request.Context(), wikaeval.CreateDatasetInput{
		ActorID:     userID,
		TenantID:    tenantID,
		KBID:        strings.TrimSpace(c.Param("id")),
		Name:        name,
		Description: strings.TrimSpace(req.Description),
	})
	if err != nil {
		c.Error(apperrors.NewInternalServerError("failed to create evaluation dataset"))
		return
	}
	c.JSON(http.StatusOK, result)
}

// AddQAItem 添加一条黄金 QA。
func (h *WikaEvaluationHandler) AddQAItem(c *gin.Context) {
	_, _, ok := wikaKnowledgeContext(c)
	if !ok {
		return
	}
	if h.service == nil {
		c.Error(apperrors.NewInternalServerError("wika evaluation service unavailable"))
		return
	}
	datasetID, err := strconv.ParseUint(strings.TrimSpace(c.Param("dataset_id")), 10, 64)
	if err != nil || datasetID == 0 {
		c.Error(apperrors.NewBadRequestError("invalid dataset id"))
		return
	}
	var req addWikaEvalQAItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewBadRequestError("invalid request body").WithDetails(err.Error()))
		return
	}
	result, err := h.service.AddQAItem(c.Request.Context(), wikaeval.AddQAItemInput{
		DatasetID:            datasetID,
		Question:             strings.TrimSpace(req.Question),
		ExpectedAnswer:       strings.TrimSpace(req.ExpectedAnswer),
		ExpectedKnowledgeIDs: req.ExpectedKnowledgeIDs,
		ExpectedChunkIDs:     req.ExpectedChunkIDs,
		Tags:                 req.Tags,
		Enabled:              req.Enabled,
	})
	if err != nil {
		if err == wikaeval.ErrInvalidQAItem {
			c.Error(apperrors.NewBadRequestError("question is required"))
			return
		}
		c.Error(apperrors.NewInternalServerError("failed to add evaluation qa item"))
		return
	}
	c.JSON(http.StatusOK, result)
}

// RunEvaluation 触发一次正式评测 run。
func (h *WikaEvaluationHandler) RunEvaluation(c *gin.Context) {
	userID, tenantID, ok := wikaKnowledgeContext(c)
	if !ok {
		return
	}
	if h.service == nil {
		c.Error(apperrors.NewInternalServerError("wika evaluation service unavailable"))
		return
	}
	var req runWikaEvalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewBadRequestError("invalid request body").WithDetails(err.Error()))
		return
	}
	if req.DatasetID == 0 {
		c.Error(apperrors.NewBadRequestError("dataset_id is required"))
		return
	}
	result, err := h.service.RunEvaluation(c.Request.Context(), wikaeval.RunInput{
		ActorID:   userID,
		TenantID:  tenantID,
		KBID:      strings.TrimSpace(c.Param("id")),
		DatasetID: req.DatasetID,
	})
	if err != nil {
		if err == wikaeval.ErrDatasetNotReadyForFormalRun {
			c.Error(apperrors.NewBadRequestError("formal evaluation requires expected knowledge or chunk ids"))
			return
		}
		c.Error(apperrors.NewInternalServerError("failed to run evaluation"))
		return
	}
	c.JSON(http.StatusOK, result)
}
