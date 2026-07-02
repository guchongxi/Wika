package handler

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	wikaauth "github.com/Tencent/WeKnora/internal/wika/auth"
)

const maxWikaTokenTTL = 90 * 24 * time.Hour

var allowedWikaTokenScopes = map[string]struct{}{
	"knowledge:push":    {},
	"knowledge:search":  {},
	"knowledge:read":    {},
	"suggestion:create": {},
	"mcp:admin":         {},
}

type wikaTokenService interface {
	CreateToken(ctx context.Context, input wikaauth.CreateTokenInput) (*wikaauth.CreatedToken, error)
	ListTokens(ctx context.Context, userID string, tenantID uint64) ([]*types.WikaUserToken, error)
	RevokeToken(ctx context.Context, userID string, tokenID uint64) error
	ListUsage(ctx context.Context, filter wikaauth.TokenUsageFilter) (*wikaauth.TokenUsageListResult, error)
	ListUsageEvents(ctx context.Context, filter wikaauth.TokenUsageFilter) (*wikaauth.TokenUsageEventListResult, error)
}

// WikaTokenHandler 暴露日常 MCP 工具使用的用户级 PAT 管理接口。
type WikaTokenHandler struct {
	service wikaTokenService
	now     func() time.Time
}

// NewWikaTokenHandler 创建 Wika token handler。
func NewWikaTokenHandler(service *wikaauth.TokenService) *WikaTokenHandler {
	return &WikaTokenHandler{service: service, now: time.Now}
}

type createWikaTokenRequest struct {
	Name      string    `json:"name"`
	TenantID  *uint64   `json:"tenant_id,omitempty"`
	Scopes    []string  `json:"scopes"`
	ExpiresAt time.Time `json:"expires_at"`
}

type wikaTokenResponse struct {
	ID          uint64     `json:"id"`
	Name        string     `json:"name"`
	Token       string     `json:"token,omitempty"`
	TokenPrefix string     `json:"token_prefix"`
	Scopes      []string   `json:"scopes"`
	ExpiresAt   time.Time  `json:"expires_at"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at,omitempty"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
}

// CreateToken 创建用户级 PAT，并且只在本次响应返回明文 token。
func (h *WikaTokenHandler) CreateToken(c *gin.Context) {
	userID, tenantID, ok := wikaTokenContext(c)
	if !ok {
		return
	}
	var req createWikaTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewBadRequestError("invalid request body").WithDetails(err.Error()))
		return
	}
	name := strings.TrimSpace(req.Name)
	scopes, err := normalizeWikaTokenScopes(req.Scopes)
	if err != nil {
		c.Error(apperrors.NewBadRequestError(err.Error()))
		return
	}
	if hasWikaTokenScope(scopes, "mcp:admin") && !isWikaMCPAdminActor(c.Request.Context()) {
		c.Error(apperrors.NewForbiddenError("mcp:admin scope requires tenant admin"))
		return
	}
	targetTenantID := tenantID
	if req.TenantID != nil {
		if *req.TenantID == 0 || *req.TenantID != tenantID {
			c.Error(apperrors.NewBadRequestError("tenant_id must match current tenant"))
			return
		}
		targetTenantID = *req.TenantID
	}
	now := h.currentTime()
	if name == "" {
		c.Error(apperrors.NewBadRequestError("name is required"))
		return
	}
	if req.ExpiresAt.IsZero() || !req.ExpiresAt.After(now) {
		c.Error(apperrors.NewBadRequestError("expires_at must be in the future"))
		return
	}
	if req.ExpiresAt.After(now.Add(maxWikaTokenTTL)) {
		c.Error(apperrors.NewBadRequestError("expires_at must be within 90 days"))
		return
	}

	created, err := h.service.CreateToken(c.Request.Context(), wikaauth.CreateTokenInput{
		UserID:      userID,
		TenantID:    targetTenantID,
		Name:        name,
		Scopes:      scopes,
		ExpiresAt:   req.ExpiresAt,
		CreatedByIP: c.ClientIP(),
	})
	if err != nil {
		logger.Error(c.Request.Context(), "failed to create Wika token", err)
		c.Error(apperrors.NewInternalServerError("failed to create token"))
		return
	}
	if created == nil || created.Metadata == nil {
		c.Error(apperrors.NewInternalServerError("failed to create token"))
		return
	}
	resp := buildWikaTokenResponse(created.Metadata)
	resp.Token = created.Plaintext
	c.JSON(http.StatusCreated, resp)
}

// ListTokens 列出当前用户的 PAT 元数据，不返回明文或 hash。
func (h *WikaTokenHandler) ListTokens(c *gin.Context) {
	userID, tenantID, ok := wikaTokenContext(c)
	if !ok {
		return
	}
	tokens, err := h.service.ListTokens(c.Request.Context(), userID, tenantID)
	if err != nil {
		logger.Error(c.Request.Context(), "failed to list Wika tokens", err)
		c.Error(apperrors.NewInternalServerError("failed to list tokens"))
		return
	}
	resp := make([]wikaTokenResponse, 0, len(tokens))
	for _, token := range tokens {
		if token == nil {
			continue
		}
		resp = append(resp, buildWikaTokenResponse(token))
	}
	c.JSON(http.StatusOK, gin.H{"tokens": resp})
}

// AuthorizeMCPAdminToolset 校验当前用户能否在 Remote MCP 中启用管理工具集。
func (h *WikaTokenHandler) AuthorizeMCPAdminToolset(c *gin.Context) {
	if _, _, ok := wikaTokenContext(c); !ok {
		return
	}
	if !isWikaMCPAdminActor(c.Request.Context()) {
		c.Error(apperrors.NewForbiddenError("admin MCP toolset requires tenant admin"))
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"allowed": true,
		"toolset": "admin",
	})
}

// ListTokenUsage 列出当前用户或当前租户内全部用户的 Wika PAT 调用统计。
func (h *WikaTokenHandler) ListTokenUsage(c *gin.Context) {
	userID, tenantID, ok := wikaTokenContext(c)
	if !ok {
		return
	}
	if h.service == nil {
		c.Error(apperrors.NewInternalServerError("wika token service unavailable"))
		return
	}
	filter, ok := h.parseUsageFilter(c, userID, tenantID)
	if !ok {
		return
	}
	result, err := h.service.ListUsage(c.Request.Context(), filter)
	if err != nil {
		logger.Error(c.Request.Context(), "failed to list Wika token usage", err)
		c.Error(apperrors.NewInternalServerError("failed to list token usage"))
		return
	}
	if result == nil {
		result = &wikaauth.TokenUsageListResult{Scope: filter.Scope, Items: []wikaauth.TokenUsageItem{}}
	}
	c.JSON(http.StatusOK, result)
}

// ListTokenUsageEvents 列出 Wika PAT 最近调用事件。
func (h *WikaTokenHandler) ListTokenUsageEvents(c *gin.Context) {
	userID, tenantID, ok := wikaTokenContext(c)
	if !ok {
		return
	}
	if h.service == nil {
		c.Error(apperrors.NewInternalServerError("wika token service unavailable"))
		return
	}
	filter, ok := h.parseUsageFilter(c, userID, tenantID)
	if !ok {
		return
	}
	result, err := h.service.ListUsageEvents(c.Request.Context(), filter)
	if err != nil {
		logger.Error(c.Request.Context(), "failed to list Wika token usage events", err)
		c.Error(apperrors.NewInternalServerError("failed to list token usage events"))
		return
	}
	if result == nil {
		result = &wikaauth.TokenUsageEventListResult{Scope: filter.Scope, Events: []wikaauth.TokenUsageEventItem{}}
	}
	c.JSON(http.StatusOK, result)
}

// RevokeToken 撤销当前用户自己的 PAT。
func (h *WikaTokenHandler) RevokeToken(c *gin.Context) {
	userID, _, ok := wikaTokenContext(c)
	if !ok {
		return
	}
	tokenID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || tokenID == 0 {
		c.Error(apperrors.NewBadRequestError("invalid token id"))
		return
	}
	if err := h.service.RevokeToken(c.Request.Context(), userID, tokenID); err != nil {
		if stderrors.Is(err, wikaauth.ErrTokenInvalid) {
			c.Error(apperrors.NewNotFoundError("token not found"))
			return
		}
		logger.Error(c.Request.Context(), "failed to revoke Wika token", err)
		c.Error(apperrors.NewInternalServerError("failed to revoke token"))
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *WikaTokenHandler) currentTime() time.Time {
	if h.now != nil {
		return h.now()
	}
	return time.Now()
}

func (h *WikaTokenHandler) parseUsageFilter(c *gin.Context, userID string, tenantID uint64) (wikaauth.TokenUsageFilter, bool) {
	now := h.currentTime()
	scope := strings.TrimSpace(c.Query("scope"))
	if scope == "" {
		scope = wikaauth.UsageScopeMine
	}
	if scope != wikaauth.UsageScopeMine && scope != wikaauth.UsageScopeAll {
		c.Error(apperrors.NewBadRequestError("scope must be mine or all"))
		return wikaauth.TokenUsageFilter{}, false
	}
	role := types.TenantRoleFromContext(c.Request.Context())
	if scope == wikaauth.UsageScopeAll && !role.HasPermission(types.TenantRoleAdmin) {
		c.Error(apperrors.NewForbiddenError("scope=all requires tenant admin"))
		return wikaauth.TokenUsageFilter{}, false
	}

	ownerUserID := strings.TrimSpace(c.Query("owner_user_id"))
	if scope == wikaauth.UsageScopeMine {
		if ownerUserID != "" && ownerUserID != userID {
			c.Error(apperrors.NewForbiddenError("owner_user_id must match current user"))
			return wikaauth.TokenUsageFilter{}, false
		}
		ownerUserID = ""
	}
	tokenID, ok := parseWikaTokenOptionalUint64(c, "token_id")
	if !ok {
		return wikaauth.TokenUsageFilter{}, false
	}
	limit, ok := parseWikaTokenOptionalInt(c, "limit", 50)
	if !ok {
		return wikaauth.TokenUsageFilter{}, false
	}
	offset, ok := parseWikaTokenOptionalInt(c, "offset", 0)
	if !ok {
		return wikaauth.TokenUsageFilter{}, false
	}
	from, ok := parseWikaTokenOptionalDate(c, "from")
	if !ok {
		return wikaauth.TokenUsageFilter{}, false
	}
	to, ok := parseWikaTokenOptionalDate(c, "to")
	if !ok {
		return wikaauth.TokenUsageFilter{}, false
	}
	if from.IsZero() {
		from = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -6)
	}
	if to.IsZero() {
		to = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	}
	if from.After(to) {
		c.Error(apperrors.NewBadRequestError("from must be before or equal to to"))
		return wikaauth.TokenUsageFilter{}, false
	}
	if to.Sub(from) > 90*24*time.Hour {
		c.Error(apperrors.NewBadRequestError("date range must be within 90 days"))
		return wikaauth.TokenUsageFilter{}, false
	}
	var success *bool
	if raw := strings.TrimSpace(c.Query("success")); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			c.Error(apperrors.NewBadRequestError("success must be true or false"))
			return wikaauth.TokenUsageFilter{}, false
		}
		success = &parsed
	}
	return wikaauth.TokenUsageFilter{
		TenantID:     tenantID,
		ViewerUserID: userID,
		Scope:        scope,
		OwnerUserID:  ownerUserID,
		TokenID:      tokenID,
		ToolName:     strings.TrimSpace(c.Query("tool_name")),
		Success:      success,
		From:         from,
		To:           to,
		Limit:        limit,
		Offset:       offset,
		Now:          now,
	}, true
}

func parseWikaTokenOptionalUint64(c *gin.Context, key string) (uint64, bool) {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return 0, true
	}
	parsed, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || parsed == 0 {
		c.Error(apperrors.NewBadRequestError(key + " must be a positive integer"))
		return 0, false
	}
	return parsed, true
}

func parseWikaTokenOptionalInt(c *gin.Context, key string, fallback int) (int, bool) {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return fallback, true
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed < 0 {
		c.Error(apperrors.NewBadRequestError(key + " must be a non-negative integer"))
		return 0, false
	}
	return parsed, true
}

func parseWikaTokenOptionalDate(c *gin.Context, key string) (time.Time, bool) {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return time.Time{}, true
	}
	parsed, err := time.Parse("2006-01-02", raw)
	if err != nil {
		c.Error(apperrors.NewBadRequestError(key + " must use YYYY-MM-DD"))
		return time.Time{}, false
	}
	return parsed, true
}

func wikaTokenContext(c *gin.Context) (string, uint64, bool) {
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

func normalizeWikaTokenScopes(raw []string) ([]string, error) {
	if len(raw) == 0 {
		return nil, stderrors.New("scopes are required")
	}
	scopes := make([]string, 0, len(raw))
	seen := map[string]struct{}{}
	for _, scope := range raw {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			return nil, stderrors.New("scope must not be empty")
		}
		if _, ok := allowedWikaTokenScopes[scope]; !ok {
			return nil, stderrors.New("unsupported scope: " + scope)
		}
		if _, ok := seen[scope]; ok {
			continue
		}
		seen[scope] = struct{}{}
		scopes = append(scopes, scope)
	}
	return scopes, nil
}

func hasWikaTokenScope(scopes []string, target string) bool {
	for _, scope := range scopes {
		if scope == target {
			return true
		}
	}
	return false
}

func isWikaMCPAdminActor(ctx context.Context) bool {
	if types.TenantRoleFromContext(ctx).HasPermission(types.TenantRoleAdmin) {
		return true
	}
	if types.IsSystemAdminFromContext(ctx) {
		return true
	}
	user, _ := ctx.Value(types.UserContextKey).(*types.User)
	return user != nil && user.IsSystemAdmin
}

func buildWikaTokenResponse(token *types.WikaUserToken) wikaTokenResponse {
	return wikaTokenResponse{
		ID:          token.ID,
		Name:        token.Name,
		TokenPrefix: token.TokenPrefix,
		Scopes:      wikaTokenScopesFromJSON(token.Scopes),
		ExpiresAt:   token.ExpiresAt,
		RevokedAt:   token.RevokedAt,
		CreatedAt:   token.CreatedAt,
		LastUsedAt:  token.LastUsedAt,
	}
}

func wikaTokenScopesFromJSON(raw types.JSON) []string {
	var scopes []string
	if err := json.Unmarshal([]byte(raw), &scopes); err != nil {
		return []string{}
	}
	return scopes
}
