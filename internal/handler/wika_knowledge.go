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
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	wikaintake "github.com/Tencent/WeKnora/internal/wika/intake"
	wikasearch "github.com/Tencent/WeKnora/internal/wika/search"
)

type wikaIntakeService interface {
	PushKnowledge(ctx context.Context, input wikaintake.PushKnowledgeInput) (*wikaintake.PushKnowledgeResult, error)
}

type wikaSearchService interface {
	SearchKnowledge(ctx context.Context, input wikasearch.SearchInput) (*wikasearch.SearchResult, error)
	ListMyKnowledge(ctx context.Context, input wikasearch.MineInput) (*wikasearch.MineResult, error)
	ExpandKnowledge(ctx context.Context, input wikasearch.ExpandInput) (*wikasearch.ExpandResult, error)
}

// WikaKnowledgeHandler 暴露 Wika 日常知识生产接口。
type WikaKnowledgeHandler struct {
	intake wikaIntakeService
	search wikaSearchService
}

// NewWikaKnowledgeHandler 创建 Wika 知识 handler。
func NewWikaKnowledgeHandler(intake *wikaintake.Service, search *wikasearch.Service) *WikaKnowledgeHandler {
	return &WikaKnowledgeHandler{intake: intake, search: search}
}

type pushWikaKnowledgeRequest struct {
	Title          string     `json:"title"`
	Content        string     `json:"content"`
	Source         string     `json:"source"`
	Tags           []string   `json:"tags"`
	Evidence       string     `json:"evidence"`
	ExpiresAt      *time.Time `json:"expires_at"`
	IdempotencyKey string     `json:"idempotency_key"`
	DryRun         bool       `json:"dry_run"`
}

type searchWikaKnowledgeRequest struct {
	Query       string `json:"query"`
	Limit       int    `json:"limit"`
	IncludeTeam *bool  `json:"include_team"`
	Format      string `json:"format"`
}

type expandWikaKnowledgeRequest struct {
	IDs []string `json:"ids"`
}

// PushKnowledge 接收 Web/MCP 知识草稿并交给统一 intake 链路。
func (h *WikaKnowledgeHandler) PushKnowledge(c *gin.Context) {
	userID, tenantID, ok := wikaKnowledgeContext(c)
	if !ok {
		return
	}
	if h.intake == nil {
		c.Error(apperrors.NewInternalServerError("wika intake service unavailable"))
		return
	}

	var req pushWikaKnowledgeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewBadRequestError("invalid request body").WithDetails(err.Error()))
		return
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		c.Error(apperrors.NewBadRequestError("content is required"))
		return
	}

	result, err := h.intake.PushKnowledge(c.Request.Context(), wikaintake.PushKnowledgeInput{
		UserID:         userID,
		TenantID:       tenantID,
		Title:          strings.TrimSpace(req.Title),
		Content:        content,
		Source:         strings.TrimSpace(req.Source),
		Tags:           req.Tags,
		Evidence:       strings.TrimSpace(req.Evidence),
		ExpiresAt:      req.ExpiresAt,
		IdempotencyKey: strings.TrimSpace(req.IdempotencyKey),
		DryRun:         req.DryRun,
	})
	if err != nil {
		if stderrors.Is(err, context.Canceled) {
			c.Error(apperrors.NewBadRequestError("request cancelled"))
			return
		}
		logger.Error(c.Request.Context(), "failed to push Wika knowledge", err)
		c.Error(apperrors.NewInternalServerError("failed to push knowledge"))
		return
	}
	if result == nil {
		c.Error(apperrors.NewInternalServerError("failed to push knowledge"))
		return
	}
	c.JSON(http.StatusOK, result)
}

// SearchKnowledge 执行 AI 友好的 compact 知识检索。
func (h *WikaKnowledgeHandler) SearchKnowledge(c *gin.Context) {
	userID, tenantID, ok := wikaKnowledgeContext(c)
	if !ok {
		return
	}
	if h.search == nil {
		c.Error(apperrors.NewInternalServerError("wika search service unavailable"))
		return
	}

	var req searchWikaKnowledgeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewBadRequestError("invalid request body").WithDetails(err.Error()))
		return
	}
	query := strings.TrimSpace(req.Query)
	if query == "" {
		c.Error(apperrors.NewBadRequestError("query is required"))
		return
	}
	includeTeam := true
	if req.IncludeTeam != nil {
		includeTeam = *req.IncludeTeam
	}
	format := strings.TrimSpace(req.Format)
	if format == "" {
		format = "compact"
	}

	result, err := h.search.SearchKnowledge(c.Request.Context(), wikasearch.SearchInput{
		UserID:      userID,
		TenantID:    tenantID,
		Query:       query,
		Limit:       req.Limit,
		IncludeTeam: includeTeam,
		Format:      format,
	})
	if err != nil {
		logger.Error(c.Request.Context(), "failed to search Wika knowledge", err)
		c.Error(apperrors.NewInternalServerError("failed to search knowledge"))
		return
	}
	if result == nil {
		result = &wikasearch.SearchResult{Results: []wikasearch.ResultItem{}}
	}
	c.JSON(http.StatusOK, result)
}

// ExpandKnowledge 按 ID 展开知识详情，service 会重新做可读 scope 过滤。
func (h *WikaKnowledgeHandler) ExpandKnowledge(c *gin.Context) {
	userID, _, ok := wikaKnowledgeContext(c)
	if !ok {
		return
	}
	if h.search == nil {
		c.Error(apperrors.NewInternalServerError("wika search service unavailable"))
		return
	}
	var req expandWikaKnowledgeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewBadRequestError("invalid request body").WithDetails(err.Error()))
		return
	}
	ids := normalizeKnowledgeIDs(req.IDs)
	if len(ids) == 0 {
		c.Error(apperrors.NewBadRequestError("ids are required"))
		return
	}
	result, err := h.search.ExpandKnowledge(c.Request.Context(), wikasearch.ExpandInput{
		UserID: userID,
		IDs:    ids,
	})
	if err != nil {
		logger.Error(c.Request.Context(), "failed to expand Wika knowledge", err)
		c.Error(apperrors.NewInternalServerError("failed to expand knowledge"))
		return
	}
	if result == nil {
		result = &wikasearch.ExpandResult{Results: []wikasearch.ExpandedItem{}}
	}
	c.JSON(http.StatusOK, result)
}

// ListMyKnowledge 列出当前用户个人默认知识库里的知识。
func (h *WikaKnowledgeHandler) ListMyKnowledge(c *gin.Context) {
	userID, _, ok := wikaKnowledgeContext(c)
	if !ok {
		return
	}
	if h.search == nil {
		c.Error(apperrors.NewInternalServerError("wika search service unavailable"))
		return
	}
	limit := 20
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			c.Error(apperrors.NewBadRequestError("limit must be a positive integer"))
			return
		}
		limit = parsed
	}
	result, err := h.search.ListMyKnowledge(c.Request.Context(), wikasearch.MineInput{
		UserID: userID,
		Limit:  limit,
		Status: strings.TrimSpace(c.Query("status")),
		Tag:    strings.TrimSpace(c.Query("tag")),
	})
	if err != nil {
		logger.Error(c.Request.Context(), "failed to list Wika personal knowledge", err)
		c.Error(apperrors.NewInternalServerError("failed to list personal knowledge"))
		return
	}
	if result == nil {
		result = &wikasearch.MineResult{Results: []wikasearch.ResultItem{}}
	}
	c.JSON(http.StatusOK, result)
}

func wikaKnowledgeContext(c *gin.Context) (string, uint64, bool) {
	userID := c.GetString(types.UserIDContextKey.String())
	if userID == "" {
		c.Error(apperrors.NewUnauthorizedError("user ID not found"))
		return "", 0, false
	}
	tenantID := c.GetUint64(types.TenantIDContextKey.String())
	if tenantID == 0 {
		c.Error(apperrors.NewUnauthorizedError("tenant ID not found"))
		return "", 0, false
	}
	return userID, tenantID, true
}

func normalizeKnowledgeIDs(raw []string) []string {
	if len(raw) == 0 {
		return nil
	}
	ids := make([]string, 0, len(raw))
	seen := map[string]struct{}{}
	for _, id := range raw {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}
