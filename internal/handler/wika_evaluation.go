package handler

import (
	"context"
	stderrors "errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
	wikaeval "github.com/Tencent/WeKnora/internal/wika/evaluation"
	"gorm.io/gorm"
)

type wikaEvaluationService interface {
	CreateDataset(ctx context.Context, input wikaeval.CreateDatasetInput) (*types.WikaEvalDataset, error)
	AddQAItem(ctx context.Context, input wikaeval.AddQAItemInput) (*types.WikaEvalQAItem, error)
	ImportDataset(ctx context.Context, input wikaeval.ImportDatasetInput) (*wikaeval.ImportDatasetResult, error)
	DryRun(ctx context.Context, input wikaeval.DryRunInput) (*wikaeval.DryRunResult, error)
	RunEvaluation(ctx context.Context, input wikaeval.RunInput) (*types.WikaEvalRun, error)
	ExportDataset(ctx context.Context, input wikaeval.ExportDatasetInput) (*wikaeval.ExportDatasetResult, error)
	Trend(ctx context.Context, input wikaeval.TrendInput) (*wikaeval.TrendResult, error)
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

type importWikaEvalDatasetRequest struct {
	Items []addWikaEvalQAItemRequest `json:"items"`
}

type runWikaEvalRequest struct {
	DatasetID uint64 `json:"dataset_id"`
}

type dryRunWikaEvalRequest struct {
	Question string `json:"question"`
	Limit    int    `json:"limit"`
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
	userID, tenantID, ok := wikaKnowledgeContext(c)
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
		ActorID:              userID,
		TenantID:             tenantID,
		KBID:                 strings.TrimSpace(c.Param("id")),
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
		if stderrors.Is(err, gorm.ErrRecordNotFound) {
			c.Error(apperrors.NewNotFoundError("evaluation dataset not found"))
			return
		}
		c.Error(apperrors.NewInternalServerError("failed to add evaluation qa item"))
		return
	}
	c.JSON(http.StatusOK, result)
}

// ImportDataset 批量导入黄金 QA。
func (h *WikaEvaluationHandler) ImportDataset(c *gin.Context) {
	userID, tenantID, ok := wikaKnowledgeContext(c)
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
	var req importWikaEvalDatasetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewBadRequestError("invalid request body").WithDetails(err.Error()))
		return
	}
	items := make([]wikaeval.ImportQAItem, 0, len(req.Items))
	for _, item := range req.Items {
		enabled := true
		if item.Enabled != nil {
			enabled = *item.Enabled
		}
		items = append(items, wikaeval.ImportQAItem{
			Question:             strings.TrimSpace(item.Question),
			ExpectedAnswer:       strings.TrimSpace(item.ExpectedAnswer),
			ExpectedKnowledgeIDs: item.ExpectedKnowledgeIDs,
			ExpectedChunkIDs:     item.ExpectedChunkIDs,
			Tags:                 item.Tags,
			Enabled:              enabled,
		})
	}
	result, err := h.service.ImportDataset(c.Request.Context(), wikaeval.ImportDatasetInput{
		ActorID:   userID,
		TenantID:  tenantID,
		KBID:      strings.TrimSpace(c.Param("id")),
		DatasetID: datasetID,
		Items:     items,
	})
	if err != nil {
		if err == wikaeval.ErrInvalidQAItem {
			c.Error(apperrors.NewBadRequestError("question is required"))
			return
		}
		if stderrors.Is(err, gorm.ErrRecordNotFound) {
			c.Error(apperrors.NewNotFoundError("evaluation dataset not found"))
			return
		}
		c.Error(apperrors.NewInternalServerError("failed to import evaluation dataset"))
		return
	}
	c.JSON(http.StatusOK, result)
}

// DryRun 执行单条 QA 召回预览，不写入正式评测指标。
func (h *WikaEvaluationHandler) DryRun(c *gin.Context) {
	userID, tenantID, ok := wikaKnowledgeContext(c)
	if !ok {
		return
	}
	if h.service == nil {
		c.Error(apperrors.NewInternalServerError("wika evaluation service unavailable"))
		return
	}
	var req dryRunWikaEvalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewBadRequestError("invalid request body").WithDetails(err.Error()))
		return
	}
	result, err := h.service.DryRun(c.Request.Context(), wikaeval.DryRunInput{
		ActorID:  userID,
		TenantID: tenantID,
		KBID:     strings.TrimSpace(c.Param("id")),
		Question: strings.TrimSpace(req.Question),
		Limit:    req.Limit,
	})
	if err != nil {
		if err == wikaeval.ErrInvalidQAItem {
			c.Error(apperrors.NewBadRequestError("question is required"))
			return
		}
		c.Error(apperrors.NewInternalServerError("failed to dry-run evaluation"))
		return
	}
	c.JSON(http.StatusOK, result)
}

// ExportDataset 导出黄金 QA 数据集，不包含标准答案。
func (h *WikaEvaluationHandler) ExportDataset(c *gin.Context) {
	userID, tenantID, ok := wikaKnowledgeContext(c)
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
	result, err := h.service.ExportDataset(c.Request.Context(), wikaeval.ExportDatasetInput{
		ActorID:   userID,
		TenantID:  tenantID,
		KBID:      strings.TrimSpace(c.Param("id")),
		DatasetID: datasetID,
	})
	if err != nil {
		if stderrors.Is(err, gorm.ErrRecordNotFound) {
			c.Error(apperrors.NewNotFoundError("evaluation dataset not found"))
			return
		}
		c.Error(apperrors.NewInternalServerError("failed to export evaluation dataset"))
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
		if stderrors.Is(err, gorm.ErrRecordNotFound) {
			c.Error(apperrors.NewNotFoundError("evaluation dataset not found"))
			return
		}
		c.Error(apperrors.NewInternalServerError("failed to run evaluation"))
		return
	}
	c.JSON(http.StatusOK, result)
}

// Trend 返回知识库最近的正式评测趋势。
func (h *WikaEvaluationHandler) Trend(c *gin.Context) {
	userID, tenantID, ok := wikaKnowledgeContext(c)
	if !ok {
		return
	}
	if h.service == nil {
		c.Error(apperrors.NewInternalServerError("wika evaluation service unavailable"))
		return
	}
	limit, err := strconv.Atoi(strings.TrimSpace(c.DefaultQuery("limit", "20")))
	if err != nil {
		c.Error(apperrors.NewBadRequestError("invalid limit"))
		return
	}
	result, err := h.service.Trend(c.Request.Context(), wikaeval.TrendInput{
		ActorID:  userID,
		TenantID: tenantID,
		KBID:     strings.TrimSpace(c.Param("id")),
		Limit:    limit,
	})
	if err != nil {
		c.Error(apperrors.NewInternalServerError("failed to load evaluation trend"))
		return
	}
	c.JSON(http.StatusOK, result)
}
